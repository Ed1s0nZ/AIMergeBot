package platform

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/xanzy/go-gitlab"
)

func (r *Runner) commentLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if r.Settings == nil || !r.Settings.Snapshot().EnableMRComment {
			continue
		}
		d, err := r.Store.claimCommentDelivery(ctx, r.owner)
		if err != nil {
			continue
		}
		attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
		r.deliverComment(attempt, d)
		cancel()
	}
}
func (r *Runner) deliveryPreflight(ctx context.Context, d CommentDelivery, run Run) bool {
	if requireLegacyRepositorySnapshot(ctx, r.Store.DB, run.Snapshot) != nil {
		return false
	}
	if r.Settings == nil || !r.Settings.Snapshot().EnableMRComment {
		return false
	}
	cfg := r.Settings.Snapshot()
	if len(contextPolicyItems(run.Snapshot)) > 0 {
		return false
	}
	if run.Status != "succeeded" || run.AuditPolicy == nil || strings.TrimRight(cfg.GitLab.URL, "/") != run.AuditPolicy.RepositoryURL {
		return false
	}
	var enabled bool
	if r.Store.DB.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, run.ProjectID).Scan(&enabled) != nil || !enabled {
		return false
	}
	if run.RequestedBy > 0 {
		if _, err := requireSnapshotRole(ctx, r.Store.DB, run.Snapshot, run.RequestedBy, "viewer"); err != nil {
			return false
		}
	}
	var reviewActor int64
	if r.Store.DB.QueryRowContext(ctx, `SELECT review_actor FROM platform_comment_delivery WHERE run_id=?`, d.RunID).Scan(&reviewActor) != nil {
		return false
	}
	if reviewActor > 0 {
		if _, err := requireSnapshotRole(ctx, r.Store.DB, run.Snapshot, reviewActor, "reviewer"); err != nil {
			return false
		}
	}
	var valid bool
	err := r.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_comment_delivery WHERE run_id=? AND state='sending'`+deliveryFence+`)`, d.RunID, d.ClaimOwner).Scan(&valid)
	return err == nil && valid
}
func (r *Runner) deliverComment(ctx context.Context, d CommentDelivery) {
	deferState := func(state, reason string, causes ...error) {
		save, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = r.Store.deferComment(save, d, state, reason, causes...)
	}
	run, err := r.Store.Run(ctx, d.RunID)
	if err != nil {
		deferState("blocked", "Audit result unavailable")
		return
	}
	if err := requireLegacyRepositorySnapshot(ctx, r.Store.DB, run.Snapshot); err != nil {
		state := "blocked"
		if d.State == "unknown" && errors.Is(err, ErrRepositoryUnavailable) {
			state = "unknown"
		}
		deferState(state, "Repository identity unavailable; verify any previous remote publication", err)
		return
	}
	if !r.deliveryPreflight(ctx, d, run) {
		deferState("blocked", "Publication disabled, origin changed or access unavailable")
		return
	}
	cfg := r.Settings.Snapshot()
	repo, err := NewGitLabRepository(cfg.GitLab.Token, cfg.GitLab.URL)
	if err != nil {
		deferState("blocked", "GitLab configuration unavailable")
		return
	}
	current, err := repo.Snapshot(ctx, run.ProjectID, run.MRIID)
	if err != nil {
		deferState(d.State, "Snapshot preflight unavailable", err)
		return
	}
	if current.HeadSHA != run.HeadSHA || current.BaseSHA != run.BaseSHA {
		deferState("stale", "Merge request commits changed")
		return
	}
	user, _, err := repo.Client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil || user == nil || user.ID <= 0 {
		deferState(d.State, "Authenticated GitLab identity unavailable", err)
		return
	}
	if d.AuthorID > 0 && d.AuthorID != user.ID {
		deferState("conflict", "GitLab author identity changed")
		return
	}
	var namespace string
	if err = r.Store.DB.QueryRowContext(ctx, `SELECT namespace FROM platform_comment_installation WHERE id=1`).Scan(&namespace); err != nil {
		deferState("blocked", "Publication identity unavailable")
		return
	}
	marker := commentMarker(namespace, run)
	if d.State == "unknown" {
		r.reconcileComment(ctx, repo, d, run, user.ID, marker, deferState)
		return
	}
	reviews, err := r.Store.Reviews(ctx, d.RunID)
	if err != nil {
		deferState("pending", "Review snapshot unavailable")
		return
	}
	body, err := renderComment(namespace, run, reviews)
	if err != nil {
		deferState("blocked", "Comment exceeds publication budget")
		return
	}
	if d.NoteID > 0 {
		discussion, _, e := repo.Client.Discussions.GetMergeRequestDiscussion(run.ProjectID, run.MRIID, d.DiscussionID, gitlab.WithContext(ctx))
		if e != nil || discussion == nil {
			if retryableError(e) {
				deferState("pending", "Discussion read temporarily unavailable", e)
			} else {
				deferState("conflict", "Previously published discussion unavailable")
			}
			return
		}
		note := deliveryNote(discussion, d.NoteID)
		if note == nil || note.Author.ID != user.ID || !strings.HasPrefix(note.Body, marker) || commentHash(note.Body) != d.BodyHash {
			deferState("conflict", "Published note changed or author does not match")
			return
		}
	}
	// Recheck local authorization and origin immediately before the external write.
	if !r.deliveryPreflight(ctx, d, run) || r.Settings.Snapshot().GitLab.Token != cfg.GitLab.Token {
		deferState("blocked", "Publication configuration or access changed")
		return
	}
	if err = r.Store.prepareCommentBody(ctx, d, commentHash(body), user.ID); err != nil {
		if errors.Is(err, ErrRepositoryUnavailable) {
			deferState("blocked", "Repository identity changed before publication", err)
		}
		return
	}
	if d.NoteID == 0 {
		discussion, _, e := repo.Client.Discussions.CreateMergeRequestDiscussion(run.ProjectID, run.MRIID, &gitlab.CreateMergeRequestDiscussionOptions{Body: gitlab.String(body)}, gitlab.WithContext(ctx))
		if e != nil || discussion == nil || discussion.ID == "" || len(discussion.Notes) != 1 {
			deferState("unknown", "Create acknowledgement unavailable; reconciliation required", e)
			return
		}
		note := discussion.Notes[0]
		if note == nil || note.ID <= 0 || note.Author.ID != user.ID || note.Body != body {
			deferState("unknown", "Create response could not be verified")
			return
		}
		_ = r.Store.acknowledgeComment(ctx, d, discussion.ID, note.ID, user.ID, commentHash(body))
		return
	}
	note, _, e := repo.Client.Discussions.UpdateMergeRequestDiscussionNote(run.ProjectID, run.MRIID, d.DiscussionID, d.NoteID, &gitlab.UpdateMergeRequestDiscussionNoteOptions{Body: gitlab.String(body)}, gitlab.WithContext(ctx))
	if e != nil || note == nil || note.ID != d.NoteID || note.Author.ID != user.ID || note.Body != body {
		deferState("unknown", "Update acknowledgement unavailable; reconciliation required", e)
		return
	}
	_ = r.Store.acknowledgeComment(ctx, d, d.DiscussionID, d.NoteID, user.ID, commentHash(body))
}
func deliveryNote(d *gitlab.Discussion, id int) *gitlab.Note {
	for _, n := range d.Notes {
		if n != nil && n.ID == id {
			return n
		}
	}
	return nil
}
func (r *Runner) reconcileComment(ctx context.Context, repo *GitLabRepository, d CommentDelivery, run Run, author int, marker string, fail func(string, string, ...error)) {
	var discussionID string
	var matched *gitlab.Note
	matches := 0
	complete := false
	for page := 1; page <= 10; page++ {
		list, response, err := repo.Client.Discussions.ListMergeRequestDiscussions(run.ProjectID, run.MRIID, &gitlab.ListMergeRequestDiscussionsOptions{Page: page, PerPage: 100}, gitlab.WithContext(ctx))
		if err != nil {
			fail("unknown", "Reconciliation read unavailable", err)
			return
		}
		for _, discussion := range list {
			if discussion == nil {
				continue
			}
			for _, note := range discussion.Notes {
				if note != nil && note.Author.ID == author && strings.HasPrefix(note.Body, marker) {
					matches++
					discussionID = discussion.ID
					matched = note
				}
			}
		}
		if response != nil && response.NextPage == 0 && len(list) < 100 {
			complete = true
			break
		}
	}
	if matches > 1 {
		fail("conflict", "Multiple matching publication notes")
		return
	}
	if !complete || matches != 1 || matched == nil {
		fail("unknown", "No unique publication confirmed within reconciliation budget")
		return
	}
	hash := commentHash(matched.Body)
	if matched.ID <= 0 || discussionID == "" || d.AttemptedHash == "" || hash != d.AttemptedHash {
		fail("conflict", "Matching note content differs from attempted publication")
		return
	}
	_ = r.Store.acknowledgeComment(ctx, d, discussionID, matched.ID, author, hash)
}
