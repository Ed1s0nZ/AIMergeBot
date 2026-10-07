package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckDispatcherDurableReceiptAndMidflightRevocation(t *testing.T) {
	for _, mode := range []string{"ack", "revoked", "stale", "unknown", "post_revoked", "post_superseded"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
				t.Fatal(err)
			}
			u, err := s.CreateUser(ctx, "requester", "a-long-password", "member")
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []int{1, 2} {
				if err = s.SaveProject(ctx, Project{ID: p, Name: "fixture", Enabled: true}); err != nil {
					t.Fatal(err)
				}
				if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(?,?,'viewer')`, p, u.ID); err != nil {
					t.Fatal(err)
				}
			}
			head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Method == "GET" {
					h := head
					if mode == "stale" {
						h = strings.Repeat("c", 40)
					}
					if mode == "revoked" {
						if _, err := s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=2 AND user_id=?`, u.ID); err != nil {
							t.Error(err)
						}
					}
					json.NewEncoder(w).Encode(map[string]any{"source_project_id": 2, "source_branch": "feature", "diff_refs": map[string]string{"head_sha": h, "base_sha": base}})
					return
				}
				posts++
				if req.URL.Path != "/api/v4/projects/2/statuses/"+head {
					t.Error(req.URL.Path)
				}
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if mode == "unknown" {
					w.WriteHeader(500)
					return
				}
				if mode == "post_revoked" {
					if _, err := s.DB.Exec(`DELETE FROM platform_project_members WHERE project_id=2 AND user_id=?`, u.ID); err != nil {
						t.Error(err)
					}
				}
				if mode == "post_superseded" {
					if _, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 3, HeadSHA: head, BaseSHA: base}, u.ID, true); err != nil {
						t.Error(err)
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"id": 43, "sha": head, "name": payload["name"], "status": payload["state"]})
			}))
			defer server.Close()
			id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 3, HeadSHA: head, BaseSHA: base, AuditPolicy: &AuditPolicy{RepositoryURL: server.URL}}, u.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.DB.Exec(`UPDATE platform_runs SET status='incomplete' WHERE id=?`, id); err != nil {
				t.Fatal(err)
			}
			if err = s.QueueRunCheck(ctx, id, 1, false); err != nil {
				t.Fatal(err)
			}
			cfg := Settings{}
			cfg.GitLab.URL = server.URL
			cfg.GitLab.Token = "fixture"
			settings := &SettingsService{}
			settings.current.Store(&cfg)
			runner := &Runner{Store: s, Settings: settings}
			claimed, err := runner.DispatchRunCheck(ctx)
			if err != nil || !claimed {
				t.Fatal(claimed, err)
			}
			var state string
			var remote int
			if err = s.DB.QueryRow(`SELECT state,remote_id FROM platform_check_deliveries WHERE run_id=?`, id).Scan(&state, &remote); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"ack": "published", "revoked": "failed", "stale": "stale", "unknown": "unknown", "post_revoked": "unknown", "post_superseded": "unknown"}[mode]
			if state != want {
				t.Fatal(state, want)
			}
			assessment, err := s.checkDeliveryAssessment(ctx, assessRunCheck(Run{ID: id, Snapshot: Snapshot{HeadSHA: head}, Status: "incomplete"}))
			if err != nil || assessment.PublicationState != want || assessment.Published != (mode == "ack") {
				t.Fatal("incorrect publication assessment", assessment, err)
			}
			if mode == "ack" && assessment.RemoteID != 43 {
				t.Fatal("missing acknowledgement identity", assessment)
			}

			if mode == "ack" && remote != 43 {
				t.Fatal("receipt not saved", remote)
			}
			if strings.HasPrefix(mode, "post_") && (posts != 1 || remote != 43 || assessment.Published) {
				t.Fatal("post-write revocation misrepresented", posts, remote, assessment)
			}
			if (mode == "revoked" || mode == "stale") && posts != 0 {
				t.Fatal("unauthorized or stale check sent", posts)
			}
			before := posts
			if claimed, _ = runner.DispatchRunCheck(ctx); claimed || posts != before {
				t.Fatal("terminal publication repeated")
			}
		})
	}
}
