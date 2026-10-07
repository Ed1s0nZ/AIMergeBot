package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitLabCheckPublicationPinsForkAndAcknowledgement(t *testing.T) {
	head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
	for _, mode := range []string{"ack", "stale_head", "stale_base", "source_changed", "bad_receipt", "transport_failure"} {
		t.Run(mode, func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Method == "GET" && req.URL.Path == "/api/v4/projects/1/merge_requests/3" {
					source := 2
					h, b := head, base
					if mode == "stale_head" {
						h = strings.Repeat("c", 40)
					}
					if mode == "stale_base" {
						b = strings.Repeat("c", 40)
					}
					if mode == "source_changed" {
						source = 4
					}
					json.NewEncoder(w).Encode(map[string]any{"source_project_id": source, "source_branch": "feature", "diff_refs": map[string]string{"head_sha": h, "base_sha": b}})
					return
				}
				if req.Method == "POST" && req.URL.Path == "/api/v4/projects/2/statuses/"+head {
					posts++
					var payload map[string]any
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if payload["state"] != "skipped" || payload["name"] != "aimangebot/run/9" || payload["ref"] != "feature" || strings.Contains(payload["description"].(string), "PRIVATE") {
						t.Error("wrong status payload", payload)
					}
					if mode == "transport_failure" {
						w.WriteHeader(500)
						return
					}
					h := head
					if mode == "bad_receipt" {
						h = base
					}
					json.NewEncoder(w).Encode(map[string]any{"id": 42, "sha": h, "name": payload["name"], "status": payload["state"]})
					return
				}
				t.Error("unexpected request", req.Method, req.URL.Path)
				w.WriteHeader(404)
			}))
			defer server.Close()
			repo, err := NewGitLabRepository("fixture", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			run := Run{ID: 9, Snapshot: Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: 3, HeadSHA: head, BaseSHA: base}, Status: "incomplete", Result: AuditResult{Summary: "PRIVATE"}}
			result := repo.PublishRunCheck(context.Background(), run, "https://audit.example/#/runs/9", false, func(context.Context) error { return nil })
			want := "unknown"
			if mode == "ack" {
				want = "published"
			}
			if strings.HasPrefix(mode, "stale") || mode == "source_changed" {
				want = "stale"
			}
			if result.State != want {
				t.Fatal(result)
			}
			if want == "stale" && posts != 0 {
				t.Fatal("stale snapshot published")
			}
			if want != "stale" && posts != 1 {
				t.Fatal("wrong post count", posts)
			}
			before := posts
			if repo.PublishRunCheck(context.Background(), run, "https://user:password@example.test", false, func(context.Context) error { return nil }).State != "failed" || posts != before {
				t.Fatal("unsafe URL published")
			}
		})
	}
}

func TestGitLabCheckAdviceAndBlockingMapping(t *testing.T) {
	for _, state := range []string{"failed", "incomplete", "high_risk", "unknown"} {
		if gitLabCheckState(CheckAssessment{State: state}, false) != "skipped" || gitLabCheckState(CheckAssessment{State: state}, true) != "failed" {
			t.Fatal(state)
		}
	}
	for state, want := range map[string]string{"completed": "success", "pending": "pending", "running": "running", "cancelled": "canceled", "skipped": "skipped"} {
		if gitLabCheckState(CheckAssessment{State: state}, false) != want {
			t.Fatal(state)
		}
	}
}
