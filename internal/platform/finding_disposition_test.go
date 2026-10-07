package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDispositionPermissionRevisionExpiryAndIndependentReview(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	viewer, err := s.CreateUser(ctx, "viewer", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(1,'fixture',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(1,?,'viewer')`, viewer.ID); err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("b", 40)
	run, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: head}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"}],"coverage_notes":[]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	report, err := s.Disposition(ctx, run, viewer.ID, "f")
	if err != nil || report.Current.Revision != 0 || report.CanManage {
		t.Fatal(report, err)
	}
	rev := int64(0)
	req := DispositionRequest{ExpectedRevision: &rev, Status: "accepted", Reason: "temporary mitigation", Owner: 1, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), HeadSHA: head}
	if _, err = s.SaveDisposition(ctx, run, viewer.ID, "f", req); !errors.Is(err, ErrProjectPermission) {
		t.Fatal("viewer mutation", err)
	}
	accepted, err := s.SaveDisposition(ctx, run, 1, "f", req)
	if err != nil || accepted.Revision != 1 {
		t.Fatal(accepted, err)
	}
	if _, err = s.SaveDisposition(ctx, run, 1, "f", req); !errors.Is(err, ErrConflict) {
		t.Fatal("stale accepted", err)
	}
	var reviews int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_reviews`).Scan(&reviews); err != nil || reviews != 0 {
		t.Fatal("disposition changed review", reviews, err)
	}
	at := time.Now().Add(2 * time.Hour)
	for i := 0; i < 2; i++ {
		if err = s.ExpireDispositions(ctx, at); err != nil {
			t.Fatal(err)
		}
	}
	report, err = s.Disposition(ctx, run, 1, "f")
	if err != nil || report.Current.Status != "open" || report.Current.Revision != 2 || len(report.History) != 2 || report.History[1].ExpiresAt == "" {
		t.Fatal(report, err)
	}
	where, args := workspaceACL(User{ID: 1, Role: "admin"})
	where += ` AND EXISTS(SELECT 1 FROM platform_finding_dispositions d JOIN platform_finding_index f ON f.run_id=d.run_id AND f.finding_id=d.finding_id WHERE d.run_id=platform_runs.id AND d.status='open' AND d.actor=0 AND d.revision>0)`
	_, total, err := s.listRuns(ctx, where, args, 1, 20)
	if err != nil || total != 1 {
		t.Fatal("expired risk missing from todo", total, err)
	}
	var events int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_notification_events WHERE kind='risk.expired'`).Scan(&events); err != nil || events != 1 {
		t.Fatal("duplicate expiry", events, err)
	}
	rev = report.Current.Revision
	req.Status = "resolved"
	req.ExpiresAt = ""
	req.ManualResolution = false
	if _, err = s.SaveDisposition(ctx, run, 1, "f", req); err != ErrDispositionInput {
		t.Fatal("unconfirmed resolution", err)
	}
	req.ManualResolution = true
	req.HeadSHA = strings.Repeat("c", 40)
	if _, err = s.SaveDisposition(ctx, run, 1, "f", req); err != ErrConflict {
		t.Fatal("wrong head", err)
	}
	req.HeadSHA = head
	if _, err = s.SaveDisposition(ctx, run, 1, "f", req); err != nil {
		t.Fatal(err)
	}
	other, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("c", 40)}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","severity":"high"}],"coverage_notes":[]}' WHERE id=?`, other); err != nil {
		t.Fatal(err)
	}
	report, err = s.Disposition(ctx, other, 1, "f")
	if err != nil || report.Current.Revision != 0 || report.Current.Status != "open" {
		t.Fatal("state inherited", report, err)
	}
}
