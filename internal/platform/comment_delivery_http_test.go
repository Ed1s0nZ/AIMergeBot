package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGitLabDeliveryCreateReconcileAndReviewUpdate(t *testing.T) {
	for _, mode := range []string{"acknowledged", "lost_acknowledgement", "lost_update_ack", "stale", "author_changed", "disabled", "retry_after", "duplicate_marker", "pagination_limit", "no_match"} {
		lost := mode == "lost_acknowledgement" || mode == "duplicate_marker" || mode == "pagination_limit" || mode == "no_match"
		t.Run(mode, func(t *testing.T) {
			var mu sync.Mutex
			body := ""
			head := "head"
			author := 7
			limited := false
			listCalls := 0
			creates, updates := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				note := func() map[string]any {
					return map[string]any{"id": 12, "body": body, "author": map[string]int{"id": 7}}
				}
				discussion := func() map[string]any { return map[string]any{"id": "discussion", "notes": []any{note()}} }
				switch {
				case r.URL.Path == "/api/v4/user":
					json.NewEncoder(w).Encode(map[string]int{"id": author})
				case strings.HasSuffix(r.URL.Path, "/versions"):
					json.NewEncoder(w).Encode([]any{map[string]any{"id": 1, "base_commit_sha": "base", "head_commit_sha": head}})
				case r.URL.Path == "/api/v4/projects/1/merge_requests/1":
					if limited {
						w.Header().Set("Retry-After", "300")
						w.WriteHeader(429)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"source_project_id": 1, "diff_refs": map[string]string{"base_sha": "base", "head_sha": head}})
				case strings.HasSuffix(r.URL.Path, "/discussions") && r.Method == "POST":
					var payload struct {
						Body string `json:"body"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					body = payload.Body
					creates++
					if lost && creates == 1 {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
						} else {
							conn.Close()
						}
						return
					}
					json.NewEncoder(w).Encode(discussion())
				case strings.HasSuffix(r.URL.Path, "/discussions"):
					listCalls++
					if mode == "duplicate_marker" {
						json.NewEncoder(w).Encode([]any{discussion(), discussion()})
						return
					}
					if mode == "no_match" {
						json.NewEncoder(w).Encode([]any{})
						return
					}
					if mode == "pagination_limit" {
						list := []any{}
						for i := 0; i < 100; i++ {
							list = append(list, map[string]any{"id": "filler", "notes": []any{}})
						}
						if listCalls == 1 {
							list[0] = discussion()
						}
						json.NewEncoder(w).Encode(list)
						return
					}
					json.NewEncoder(w).Encode([]any{discussion()})
				case strings.HasSuffix(r.URL.Path, "/discussions/discussion"):
					json.NewEncoder(w).Encode(discussion())
				case strings.HasSuffix(r.URL.Path, "/notes/12") && r.Method == "PUT":
					var payload struct {
						Body string `json:"body"`
					}
					json.NewDecoder(r.Body).Decode(&payload)
					body = payload.Body
					updates++
					if mode == "lost_update_ack" && updates == 1 {
						conn, _, e := w.(http.Hijacker).Hijack()
						if e != nil {
							t.Error(e)
						} else {
							conn.Close()
						}
						return
					}
					json.NewEncoder(w).Encode(note())
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			s := testStore(t)
			ctx := context.Background()
			if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			svc, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
			if err != nil {
				t.Fatal(err)
			}
			cfg := svc.Snapshot()
			cfg.GitLab.URL = server.URL
			cfg.EnableMRComment = true
			if err = svc.Save(cfg); err != nil {
				t.Fatal(err)
			}
			id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{RepositoryURL: server.URL}}, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f", Title: "synthetic", Confidence: "candidate"}}}, nil); err != nil {
				t.Fatal(err)
			}
			if err = s.AcquireWorkerInstance(ctx, "owner"); err != nil {
				t.Fatal(err)
			}
			runner := &Runner{Store: s, Settings: svc, owner: "owner"}
			deliver := func() {
				t.Helper()
				d, e := s.claimCommentDelivery(ctx, "owner")
				if e != nil {
					t.Fatal(e)
				}
				runner.deliverComment(ctx, d)
			}
			deliver()
			if lost {
				d, e := s.CommentDelivery(ctx, id)
				if e != nil || d.State != "unknown" {
					t.Fatalf("ambiguous create %+v %v", d, e)
				}
				if _, err = s.DB.Exec(`UPDATE platform_comment_delivery SET retry_at='' WHERE run_id=?`, id); err != nil {
					t.Fatal(err)
				}
				deliver()
			}
			if mode == "duplicate_marker" || mode == "pagination_limit" || mode == "no_match" {
				got, e := s.CommentDelivery(ctx, id)
				want := "unknown"
				if mode == "duplicate_marker" {
					want = "conflict"
				}
				if e != nil || got.State != want {
					t.Fatalf("reconciliation %+v %v", got, e)
				}
				if mode == "pagination_limit" && listCalls != 10 {
					t.Fatal("pagination budget", listCalls)
				}
				if mode == "no_match" {
					for i := 0; i < 3; i++ {
						s.DB.Exec(`UPDATE platform_comment_delivery SET retry_at='' WHERE run_id=?`, id)
						deliver()
					}
					s.DB.Exec(`UPDATE platform_comment_delivery SET retry_at='' WHERE run_id=?`, id)
					if _, e = s.claimCommentDelivery(ctx, "owner"); e != sql.ErrNoRows {
						t.Fatal("unbounded retries", e)
					}
				}
				if creates != 1 || updates != 0 {
					t.Fatal("ambiguous send repeated")
				}
				return
			}
			d, err := s.CommentDelivery(ctx, id)
			if err != nil || d.State != "sent" || d.NoteID != 12 || d.DiscussionID != "discussion" {
				t.Fatalf("delivery %+v %v", d, err)
			}
			// Trusted system review fixture avoids adding unrelated ACL setup.
			if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "false_positive", Reason: "synthetic guard", Actor: 0})); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			switch mode {
			case "stale":
				head = "new-head"
			case "author_changed":
				author = 8
			case "retry_after":
				limited = true
			}
			mu.Unlock()
			if mode == "disabled" {
				cfg = svc.Snapshot()
				cfg.EnableMRComment = false
				if err = svc.Save(cfg); err != nil {
					t.Fatal(err)
				}
			}
			deliver()
			if mode == "stale" || mode == "author_changed" || mode == "disabled" || mode == "retry_after" {
				got, e := s.CommentDelivery(ctx, id)
				want := map[string]string{"stale": "stale", "author_changed": "conflict", "disabled": "blocked", "retry_after": "pending"}[mode]
				if e != nil || got.State != want {
					t.Fatalf("preflight %+v %v", got, e)
				}
				if mode == "retry_after" {
					var at string
					s.DB.QueryRow(`SELECT retry_at FROM platform_comment_delivery WHERE run_id=?`, id).Scan(&at)
					until, e := time.Parse(time.RFC3339Nano, at)
					if e != nil || time.Until(until) < 290*time.Second {
						t.Fatal("ignored Retry-After", at, e)
					}
				}
				if creates != 1 || updates != 0 {
					t.Fatal("preflight allowed write")
				}
				return
			}
			if mode == "lost_update_ack" {
				s.DB.Exec(`UPDATE platform_comment_delivery SET retry_at='' WHERE run_id=?`, id)
				deliver()
				got, e := s.CommentDelivery(ctx, id)
				if e != nil || got.State != "sent" || got.SentGeneration != 2 {
					t.Fatalf("update reconciliation %+v %v", got, e)
				}
			}
			mu.Lock()
			if creates != 1 || updates != 1 || !strings.Contains(body, "误报") {
				t.Fatalf("creates%d updates%d body%s", creates, updates, body)
			}
			body += " human edit"
			mu.Unlock()
			if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "fixed", Actor: 0})); err != nil {
				t.Fatal(err)
			}
			deliver()
			d, err = s.CommentDelivery(ctx, id)
			if err != nil || d.State != "conflict" {
				t.Fatalf("human edit overwritten %+v %v", d, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if updates != 1 {
				t.Fatal("edited human content")
			}
		})
	}
}
