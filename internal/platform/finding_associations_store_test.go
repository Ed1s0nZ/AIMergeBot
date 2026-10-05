package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func persistAssociationRun(t *testing.T, s *Store, r Run) Run {
	t.Helper()
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, r.Snapshot, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, id, r.Status, "", r.Result, nil); err != nil {
		t.Fatal(err)
	}
	r, err = s.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAssociationConcurrentConfirmationAndOneToOneConflict(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	current, old := associationFixture()
	persistAssociationRun(t, s, old)
	old.HeadSHA = strings.Repeat("f", 40)
	persistAssociationRun(t, s, old)
	current = persistAssociationRun(t, s, current)
	list, err := s.FindingAssociations(ctx, current.ID, admin.ID)
	if err != nil || len(list.Items) != 2 {
		t.Fatal("two version choices missing", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, candidate := range list.Items {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := s.SaveFindingAssociation(ctx, current.ID, admin.ID, id, AssociationDecisionRequest{Decision: "confirmed", Reason: "concurrent inspection"})
			results <- err
		}(candidate.ID)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("multiple confirmed prior identities", wins, conflicts)
	}
	list, err = s.FindingAssociations(ctx, current.ID, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	var winner, other FindingAssociation
	for _, item := range list.Items {
		if item.State.Decision == "confirmed" {
			winner = item
		} else {
			other = item
		}
	}
	if _, err = s.SaveFindingAssociation(ctx, current.ID, admin.ID, winner.ID, AssociationDecisionRequest{Decision: "pending", ExpectedRevision: winner.State.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveFindingAssociation(ctx, current.ID, admin.ID, other.ID, AssociationDecisionRequest{Decision: "confirmed", Reason: "explicit replacement after withdrawal"}); err != nil {
		t.Fatal(err)
	}
}

func TestAssociationHistoryAndInputBoundsRemainVisible(t *testing.T) {
	s, admin, _ := accessFixture(t)
	ctx := context.Background()
	current, old := associationFixture()
	for i := 0; i < 22; i++ {
		old.HeadSHA = fmt.Sprintf("%040x", i+10)
		persistAssociationRun(t, s, old)
	}
	current = persistAssociationRun(t, s, current)
	list, err := s.FindingAssociations(ctx, current.ID, admin.ID)
	if err != nil || !list.Truncated || len(list.Items) != 20 {
		t.Fatal("historical task boundary hidden", len(list.Items), err)
	}
	item := list.Items[0]
	for i := 0; i < 22; i++ {
		d, err := s.SaveFindingAssociation(ctx, current.ID, admin.ID, item.ID, AssociationDecisionRequest{Decision: "rejected", Reason: "manual inspection record", ExpectedRevision: item.State.Revision})
		if err != nil {
			t.Fatal(err)
		}
		item.State = d
	}
	list, err = s.FindingAssociations(ctx, current.ID, admin.ID)
	if err != nil || len(list.Items[0].History) != 20 || !list.Items[0].HistoryTruncated || list.Items[0].State.Revision != item.State.Revision {
		t.Fatal("decision history unbounded or newest state lost", err)
	}
	raw, _ := json.Marshal(AuditResult{Findings: []Finding{}, Summary: strings.Repeat("x", associationResultBytes+1)})
	if _, err = s.DB.ExecContext(ctx, `UPDATE platform_runs SET result_json=? WHERE id=?`, string(raw), item.PriorRunID); err != nil {
		t.Fatal(err)
	}
	list, err = s.FindingAssociations(ctx, current.ID, admin.ID)
	if err != nil || !list.Truncated || len(list.Items) != 19 {
		t.Fatal("oversized prior not omitted", len(list.Items), err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE platform_runs SET result_json=? WHERE id=?`, string(raw), current.ID); err != nil {
		t.Fatal(err)
	}
	list, err = s.FindingAssociations(ctx, current.ID, admin.ID)
	if err != nil || !list.Truncated || len(list.Items) != 0 || len(list.Limitations) == 0 {
		t.Fatal("oversized current not bounded", err)
	}
}
func TestAssociationDecisionsAppendHistoryAndNeverInheritRiskReview(t *testing.T) {
	s, admin, member := accessFixture(t)
	ctx := context.Background()
	current, old := associationFixture()
	old = persistAssociationRun(t, s, old)
	current = persistAssociationRun(t, s, current)
	if err := s.SetProjectMember(ctx, 1, member.ID, "reviewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: old.ID, FindingID: "old", Status: "fixed", Reason: "old version only", Actor: admin.ID})); err != nil {
		t.Fatal(err)
	}
	list, err := s.FindingAssociations(ctx, current.ID, member.ID)
	if err != nil || len(list.Items) != 1 {
		t.Fatal("candidate not returned", list, err)
	}
	id := list.Items[0].ID
	commentBefore, err := s.CommentDelivery(ctx, current.ID)
	if err != nil {
		t.Fatal("comment queue fixture missing", err)
	}
	confirmed, err := s.SaveFindingAssociation(ctx, current.ID, member.ID, id, AssociationDecisionRequest{Decision: "confirmed", Reason: "Inspected both fixed versions"})
	if err != nil || confirmed.Revision <= 0 {
		t.Fatal("confirmation failed", err)
	}
	if _, err = s.SaveFindingAssociation(ctx, current.ID, member.ID, id, AssociationDecisionRequest{Decision: "rejected", Reason: "stale client"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision overwrote", err)
	}
	if _, err = s.SaveFindingAssociation(ctx, current.ID, member.ID, id, AssociationDecisionRequest{Decision: "pending", ExpectedRevision: confirmed.Revision}); err != nil {
		t.Fatal(err)
	}
	list, err = s.FindingAssociations(ctx, current.ID, member.ID)
	if err != nil || len(list.Items[0].History) != 2 || list.Items[0].State.Decision != "pending" || list.Items[0].History[1].Decision != "confirmed" {
		t.Fatal("history lost", list, err)
	}
	reviews, err := s.Reviews(ctx, current.ID)
	if err != nil || len(reviews) != 0 {
		t.Fatal("old risk review inherited", err)
	}
	again, err := s.Run(ctx, current.ID)
	if err != nil || again.Result.Findings[0].Fingerprint != current.Result.Findings[0].Fingerprint {
		t.Fatal("finding rewritten", err)
	}
	commentAfter, err := s.CommentDelivery(ctx, current.ID)
	if err != nil || !reflect.DeepEqual(commentBefore, commentAfter) {
		t.Fatal("association changed comment queue", err)
	}
	if _, err = s.SaveFindingAssociation(ctx, current.ID, member.ID, "forged", AssociationDecisionRequest{Decision: "confirmed", Reason: "bad id"}); !errors.Is(err, ErrConflict) {
		t.Fatal("forged association accepted", err)
	}
	if _, err = s.SaveFindingAssociation(ctx, current.ID, member.ID, id, AssociationDecisionRequest{Decision: "confirmed", Reason: " "}); !errors.Is(err, ErrAssociationDecision) {
		t.Fatal("blank reason accepted", err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveFindingAssociation(ctx, current.ID, member.ID, id, AssociationDecisionRequest{Decision: "rejected", Reason: "no permission", ExpectedRevision: list.Items[0].State.Revision}); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("viewer wrote decision", err)
	}
	if err = s.SetProjectMember(ctx, 1, member.ID, "", admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.FindingAssociations(ctx, current.ID, member.ID); !errors.Is(err, ErrProjectPermission) && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("revoked reader accessed history", err)
	}
}
