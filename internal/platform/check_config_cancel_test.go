package platform

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowPolicyChangeCancelsChecksAndFencesSender(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for _, mr := range []int{1, 2} {
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: mr, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		if err = s.QueueRunCheck(ctx, id, 1, false); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	d, err := s.ClaimRunCheck(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveWorkflowPolicy(ctx, 1, 1, 0, WorkflowPolicy{}); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		var state, code, lease string
		if err = s.DB.QueryRow(`SELECT state,code,lease FROM platform_check_deliveries WHERE run_id=?`, id).Scan(&state, &code, &lease); err != nil {
			t.Fatal(err)
		}
		want := "cancelled"
		if i == 0 {
			want = "unknown"
		}
		if state != want || code != "configuration_changed" || lease != "" {
			t.Fatal(state, code, lease)
		}
	}
	if _, err = s.CheckPublicationRun(ctx, d); err != ErrConflict {
		t.Fatal("old sender passed preflight", err)
	}
	if err = s.FinishRunCheck(ctx, d, CheckPublication{State: "published", RemoteID: 42}); err != ErrConflict {
		t.Fatal("old acknowledgement accepted", err)
	}
}
