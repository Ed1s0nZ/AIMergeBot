package platform

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAuthLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "", ""); err == nil {
		t.Fatal("bootstrap accepted missing password")
	}
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	u, token, err := s.Login(ctx, "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = s.DB.QueryRow(`SELECT token_hash FROM platform_sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token {
		t.Fatal("raw session stored")
	}
	if _, err = s.Session(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateUser(ctx, u.ID, "member", false, ""); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("last admin: %v", err)
	}
	member, err := s.CreateUser(ctx, "member", "a-member-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	_, memberToken, err := s.Login(ctx, "member", "a-member-password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateUser(ctx, member.ID, "member", true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Session(ctx, memberToken); !errors.Is(err, ErrCredentials) {
		t.Fatalf("disabled session survived: %v", err)
	}
	if _, _, err = s.Login(ctx, "member", "a-member-password"); !errors.Is(err, ErrCredentials) {
		t.Fatal("disabled login accepted")
	}
	if err = s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Session(ctx, token); !errors.Is(err, ErrCredentials) {
		t.Fatal("logout did not revoke")
	}
}

func TestRunStateAndReviewIsolation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 2, BaseSHA: "base", HeadSHA: "head"}
	id, created, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil || !created {
		t.Fatalf("enqueue: %v", err)
	}
	duplicate, created, err := s.Enqueue(ctx, snap, 1, true)
	if err != nil || created || duplicate != id {
		t.Fatalf("active duplicate: %v", err)
	}
	claimed, err := s.Claim(ctx)
	if err != nil || claimed != id {
		t.Fatalf("claim: %v", err)
	}
	result := AuditResult{Findings: []Finding{{ID: "f1"}}}
	if err = s.Finish(ctx, id, "succeeded", "", result, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveReview(ctx, Review{RunID: id, FindingID: "f1", Status: "false_positive", Reason: "guard exists", Actor: 1}); err != nil {
		t.Fatal(err)
	}
	retry, created, err := s.Enqueue(ctx, snap, 1, true)
	if err != nil || !created || retry == id {
		t.Fatalf("force retry: %v", err)
	}
	reviews, err := s.Reviews(ctx, id)
	if err != nil || len(reviews) != 1 || reviews[0].Reason != "guard exists" {
		t.Fatal("review overwritten")
	}
	snap.HeadSHA = "new-head"
	if _, created, err = s.Enqueue(ctx, snap, 1, false); err != nil || !created {
		t.Fatal("new commit skipped")
	}
	if err = s.Cancel(ctx, retry, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.Cancel(ctx, retry, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("cancel terminal succeeded")
	}
	claimed, err = s.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, claimed)
	if err != nil || r.Status != "failed" {
		t.Fatal("restart falsely succeeded")
	}
}
