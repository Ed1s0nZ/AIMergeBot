package platform

import (
	"context"
	"strings"
	"testing"
)

func TestCommentDeliveryScheduledWithReviewTransaction(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f"}}}, nil); err != nil {
		t.Fatal(err)
	}
	var generation int
	var state string
	check := func(want int, wantState string) {
		t.Helper()
		if err := s.DB.QueryRow(`SELECT desired_generation,state FROM platform_comment_delivery WHERE run_id=?`, id).Scan(&generation, &state); err != nil || generation != want || state != wantState {
			t.Fatalf("gen%d state%s: %v", generation, state, err)
		}
	}
	check(1, "pending")
	if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "accepted", Actor: 1})); err != nil {
		t.Fatal(err)
	}
	check(2, "pending")
	if _, err = s.DB.Exec(`UPDATE platform_comment_delivery SET state='unknown' WHERE run_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "fixed", Actor: 1})); err != nil {
		t.Fatal(err)
	}
	check(3, "unknown") // A new review cannot clear an ambiguous prior POST.
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE platform_reviews SET status='false_positive' WHERE run_id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	check(3, "unknown")
	var first, second string
	if err = s.DB.QueryRow(`SELECT namespace FROM platform_comment_installation`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow(`SELECT namespace FROM platform_comment_installation`).Scan(&second); err != nil || first != second || len(first) != 32 {
		t.Fatal("installation identity changed", err)
	}
}

func TestCommentDeliveryGenerationRaceAndOwnerFence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f"}}}, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkerInstance(ctx, "delivery-owner"); err != nil {
		t.Fatal(err)
	}
	d, err := s.claimCommentDelivery(ctx, "delivery-owner")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "accepted", Actor: 1})); err != nil {
		t.Fatal(err)
	}
	if err = s.prepareCommentBody(ctx, d, "hash1"); err != nil {
		t.Fatal(err)
	}
	if err = s.acknowledgeComment(ctx, d, "discussion", 12, 7, "hash1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.CommentDelivery(ctx, id)
	if err != nil || got.State != "pending" || got.SentGeneration != 1 || got.DesiredGeneration != 2 {
		t.Fatalf("lost concurrent review: %+v %v", got, err)
	}
	d, err = s.claimCommentDelivery(ctx, "delivery-owner")
	if err != nil {
		t.Fatal(err)
	}
	if d.AttemptedHash != "" {
		t.Fatal("new generation retained prior attempted body")
	}
	if err = s.ReleaseWorkerInstance(ctx, "delivery-owner"); err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireWorkerInstance(ctx, "replacement"); err != nil {
		t.Fatal(err)
	}
	if err = s.acknowledgeComment(ctx, d, "discussion", 12, 7, "hash2"); err == nil {
		t.Fatal("old owner acknowledged after takeover")
	}
	recovered, err := s.claimCommentDelivery(ctx, "replacement")
	if err != nil || recovered.State != "unknown" {
		t.Fatalf("ambiguous takeover %+v %v", recovered, err)
	}
	if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "fixed", Actor: 1})); err != nil {
		t.Fatal(err)
	}
	if err = s.acknowledgeComment(ctx, recovered, "discussion", 12, 7, "hash2"); err != nil {
		t.Fatal(err)
	}
	got, err = s.CommentDelivery(ctx, id)
	if err != nil || got.State != "pending" || got.SentGeneration != 2 || got.DesiredGeneration != 3 {
		t.Fatalf("reconciliation skipped newest generation: %+v %v", got, err)
	}
}

func TestCommentBodyRetainsReviewsAndEscapesUntrustedMarkdown(t *testing.T) {
	run := Run{ID: 9, Snapshot: Snapshot{BaseSHA: "base", HeadSHA: "head"}, Result: AuditResult{Summary: "<script>@all</script>\n/close\n  /assign @all", Findings: []Finding{{ID: "f", AnchorType: "git_metadata", Side: "head", File: "`x`", Title: "[click](https://evil)", Confidence: "candidate"}}}}
	body, err := renderComment("installation", run, []Review{{FindingID: "f", Status: "false_positive", Reason: "@team **not reproduced**"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "<script>") || strings.Contains(body, "@team") || strings.Contains(body, "@all") || strings.Contains(body, "/close") || strings.Contains(body, "/assign") || strings.Contains(body, ":0") || !strings.Contains(body, "误报") || !strings.Contains(body, "candidate") {
		t.Fatal(body)
	}
	if commentMarker("installation", run) != commentMarker("installation", run) || commentMarker("installation", run) == commentMarker("other", run) {
		t.Fatal("marker identity")
	}
	run.Result.Summary = strings.Repeat("x", 70000)
	if _, err = renderComment("installation", run, nil); err == nil {
		t.Fatal("oversized body allowed")
	}
}
