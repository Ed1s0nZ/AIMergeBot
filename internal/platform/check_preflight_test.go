package platform

import (
	"context"
	"strings"
	"testing"
)

func TestCheckPublicationPreflightRechecksPermissionsAndIdentity(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "requester", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []int{1, 2, 3} {
		if err = s.SaveProject(ctx, Project{ID: p, Name: "fixture", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(?,?,'viewer')`, p, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	id, _, err := s.Enqueue(ctx, snap, u.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET status='succeeded',error='receipt failure',audit_policy_json='{"context_repositories":[{"project_id":3,"sha":"cccccccccccccccccccccccccccccccccccccccc"}]}' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueRunCheck(ctx, id, 1, false); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimRunCheck(ctx)
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CheckPublicationRun(ctx, d)
	if err != nil || run.Error != "receipt failure" || run.HeadSHA != snap.HeadSHA || run.RequestedBy != u.ID {
		t.Fatal(run, err)
	}
	bad := d
	bad.Lease = "wrong"
	if _, err = s.CheckPublicationRun(ctx, bad); err != ErrConflict {
		t.Fatal("wrong lease passed", err)
	}
	if _, err = s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=3 AND user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckPublicationRun(ctx, d); err == nil {
		t.Fatal("revoked context passed")
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(3,?,'viewer')`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckPublicationRun(ctx, d); err == nil {
		t.Fatal("disabled publication administrator passed")
	}
	if _, err = s.DB.Exec(`UPDATE platform_users SET disabled=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckPublicationRun(ctx, d); err == nil {
		t.Fatal("disabled source project passed")
	}
	if _, err = s.DB.Exec(`UPDATE platform_projects SET enabled=1 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	snap.HeadSHA = strings.Repeat("d", 40)
	if _, _, err = s.Enqueue(ctx, snap, u.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CheckPublicationRun(ctx, d); err != ErrConflict {
		t.Fatal("older claimed run passed", err)
	}
}
