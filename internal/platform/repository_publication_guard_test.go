package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func reopenPublicationGuardStore(t *testing.T, s *Store) *Store {
	t.Helper()
	var sequence int
	var name, path string
	if err := s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	if name != "main" || path == "" {
		t.Fatal("expected persistent main database", name, path)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	return reopened
}

func TestRepositoryPublicationGuardChecks(t *testing.T) {
	for _, mode := range []string{"target", "source", "context", "get", "post", "finish"} {
		t.Run(mode, func(t *testing.T) {
			s, admin, _, id, snap, _ := ownerRecommendationFixture(t)
			ctx := context.Background()
			v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2, 3)
			bind := func(project int) {
				p := bindingValue(v, "github", "https://api.example")
				if _, err := s.SaveRepositoryBinding(ctx, project, admin.ID, 0, p); err != nil {
					t.Error(err)
				}
			}
			var gets, posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" {
					gets.Add(1)
					if mode == "get" {
						bind(3)
					}
					json.NewEncoder(w).Encode(map[string]any{"source_project_id": 3, "source_branch": "feature", "diff_refs": map[string]string{"base_sha": snap.BaseSHA, "head_sha": snap.HeadSHA}})
					return
				}
				posts.Add(1)
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if mode == "post" {
					bind(3)
				}
				json.NewEncoder(w).Encode(map[string]any{"id": 43, "sha": snap.HeadSHA, "name": payload["name"], "status": payload["state"]})
			}))
			defer server.Close()
			snap.AuditPolicy.RepositoryURL = server.URL
			policy, _ := json.Marshal(snap.AuditPolicy)
			if _, err := s.DB.Exec(`UPDATE platform_runs SET status='succeeded',audit_policy_json=? WHERE id=?`, string(policy), id); err != nil {
				t.Fatal(err)
			}
			if err := s.QueueRunCheck(ctx, id, admin.ID, false); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "target":
				bind(1)
			case "source":
				bind(3)
			case "context":
				bind(2)
			}
			cfg := Settings{}
			cfg.GitLab.URL = server.URL
			cfg.GitLab.Token = "fixture"
			settings := &SettingsService{}
			settings.current.Store(&cfg)
			runner := &Runner{Store: s, Settings: settings}
			if mode == "finish" {
				delivery, err := s.ClaimRunCheck(ctx)
				if err != nil {
					t.Fatal(err)
				}
				bind(3)
				if err = s.FinishRunCheck(ctx, delivery, CheckPublication{State: "published", RemoteID: 43}); err != nil {
					t.Fatal(err)
				}
			} else {
				if claimed, err := runner.DispatchRunCheck(ctx); err != nil || !claimed {
					t.Fatal(claimed, err)
				}
			}
			if mode == "post" || mode == "finish" {
				s = reopenPublicationGuardStore(t, s)
				if _, err := s.ClaimRunCheck(ctx); !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("reopened unknown check auto claimed", err)
				}
			}
			var state, code string
			var remote int
			if err := s.DB.QueryRow(`SELECT state,code,remote_id FROM platform_check_deliveries WHERE run_id=?`, id).Scan(&state, &code, &remote); err != nil {
				t.Fatal(err)
			}
			wantState, wantRemote, wantPosts := "failed", 0, int32(0)
			if mode == "post" || mode == "finish" {
				wantState, wantRemote = "unknown", 43
			}
			if mode == "post" {
				wantPosts = 1
			}
			if state != wantState || code != "repository_unavailable" || remote != wantRemote || posts.Load() != wantPosts {
				t.Fatal(state, code, remote, posts.Load())
			}
			if (mode == "target" || mode == "source" || mode == "context" || mode == "finish") && gets.Load() != 0 {
				t.Fatal("initial guard made requests", gets.Load())
			}
		})
	}
}

func TestRepositoryPublicationGuardComments(t *testing.T) {
	for _, mode := range []string{"before", "before_unknown", "prepare", "get", "post", "put", "reconcile"} {
		t.Run(mode, func(t *testing.T) {
			s, admin, _ := accessFixture(t)
			ctx := context.Background()
			v := bindingIntegration(t, s, admin.ID, "github", "https://api.example", 1, 2)
			var bound atomic.Bool
			bind := func() {
				if bound.CompareAndSwap(false, true) {
					if _, err := s.SaveRepositoryBinding(ctx, 2, admin.ID, 0, bindingValue(v, "github", "https://api.example")); err != nil {
						t.Error(err)
					}
				}
			}
			var calls, posts, puts atomic.Int32
			var mu sync.Mutex
			body := ""
			phase := "first"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				note := func() map[string]any {
					return map[string]any{"id": 12, "body": body, "author": map[string]int{"id": 7}}
				}
				discussion := func() map[string]any { return map[string]any{"id": "discussion", "notes": []any{note()}} }
				switch {
				case r.URL.Path == "/api/v4/user":
					if mode == "get" {
						bind()
					}
					json.NewEncoder(w).Encode(map[string]int{"id": 7})
				case strings.HasSuffix(r.URL.Path, "/versions"):
					json.NewEncoder(w).Encode([]any{map[string]any{"id": 1, "base_commit_sha": "base", "head_commit_sha": "head"}})
				case r.URL.Path == "/api/v4/projects/1/merge_requests/1":
					json.NewEncoder(w).Encode(map[string]any{"source_project_id": 2, "diff_refs": map[string]string{"base_sha": "base", "head_sha": "head"}})
				case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/discussions"):
					posts.Add(1)
					var payload struct {
						Body string `json:"body"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					body = payload.Body
					if mode == "post" {
						bind()
					}
					if mode == "reconcile" && phase == "first" {
						w.WriteHeader(500)
						return
					}
					json.NewEncoder(w).Encode(discussion())
				case r.Method == "PUT":
					puts.Add(1)
					var payload struct {
						Body string `json:"body"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					body = payload.Body
					bind()
					json.NewEncoder(w).Encode(note())
				case strings.HasSuffix(r.URL.Path, "/discussions"):
					if mode == "reconcile" {
						bind()
					}
					json.NewEncoder(w).Encode([]any{discussion()})
				case strings.HasSuffix(r.URL.Path, "/discussions/discussion"):
					json.NewEncoder(w).Encode(discussion())
				default:
					t.Error("unexpected request", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			snap := Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{RepositoryURL: server.URL}}
			id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{{ID: "f", File: "file", Severity: "low"}}}, nil); err != nil {
				t.Fatal(err)
			}
			if err = s.AcquireWorkerInstance(ctx, "publication-guard"); err != nil {
				t.Fatal(err)
			}
			cfg := Settings{}
			cfg.EnableMRComment = true
			cfg.GitLab.URL = server.URL
			cfg.GitLab.Token = "fixture"
			settings := &SettingsService{}
			settings.current.Store(&cfg)
			runner := &Runner{Store: s, Settings: settings}
			if mode == "before_unknown" {
				if _, err = s.DB.Exec(`UPDATE platform_comment_delivery SET state='unknown',attempted_hash='retained' WHERE run_id=?`, id); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "before" || mode == "before_unknown" {
				bind()
			}
			deliver := func() {
				d, err := s.claimCommentDelivery(ctx, "publication-guard")
				if err != nil {
					t.Fatal(err)
				}
				if mode == "prepare" {
					bind()
					if err := s.prepareCommentBody(ctx, d, "must-not-persist", 7); !errors.Is(err, ErrRepositoryUnavailable) {
						t.Fatal("prepare did not reject changed binding", err)
					}
					if err := s.deferComment(ctx, d, "blocked", "Repository identity unavailable", ErrRepositoryUnavailable); err != nil {
						t.Fatal(err)
					}
					return
				}
				runner.deliverComment(ctx, d)
			}
			deliver()
			if mode == "put" {
				if err = s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: id, FindingID: "f", Status: "accepted", Actor: admin.ID})); err != nil {
					t.Fatal(err)
				}
				deliver()
			}
			if mode == "reconcile" {
				mu.Lock()
				phase = "second"
				mu.Unlock()
				if _, err = s.DB.Exec(`UPDATE platform_comment_delivery SET retry_at='' WHERE run_id=?`, id); err != nil {
					t.Fatal(err)
				}
				deliver()
			}
			if mode == "post" || mode == "put" || mode == "reconcile" {
				s = reopenPublicationGuardStore(t, s)
			}
			d, err := s.CommentDelivery(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			unknown := mode == "post" || mode == "put" || mode == "reconcile" || mode == "before_unknown"
			want := "blocked"
			if unknown {
				want = "unknown"
			}
			if d.State != want {
				t.Fatal(d)
			}
			if mode == "before" || mode == "before_unknown" {
				if calls.Load() != 0 || d.Attempts != 5 {
					t.Fatal("initial binding made requests or remained retryable", calls.Load(), d)
				}
			}
			if mode == "get" && posts.Load() != 0 {
				t.Fatal("midflight binding allowed post")
			}
			if mode == "prepare" && (calls.Load() != 0 || d.AttemptedHash != "" || d.AuthorID != 0 || d.Attempts != 5) {
				t.Fatal("prepare persisted attempt evidence or allowed retry", calls.Load(), d)
			}
			if mode == "post" || mode == "put" || mode == "reconcile" {
				if posts.Load() != 1 || d.NoteID != 12 || d.DiscussionID != "discussion" || d.AuthorID != 7 || d.BodyHash == "" || d.Attempts != 5 {
					t.Fatal("lost receipt or automatic retry", posts.Load(), d)
				}
				if _, err = s.claimCommentDelivery(ctx, "publication-guard"); !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("unknown auto claimed", err)
				}
			}
			if mode == "put" && puts.Load() != 1 {
				t.Fatal(puts.Load())
			}
		})
	}
}
