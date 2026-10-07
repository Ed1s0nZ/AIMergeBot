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
	for _, mode := range []string{"ack", "pipeline", "wrong_pipeline_sha", "wrong_pipeline_project", "stale_head", "stale_base", "source_changed", "bad_receipt", "transport_failure"} {
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
					response := map[string]any{"source_project_id": source, "source_branch": "feature", "diff_refs": map[string]string{"head_sha": h, "base_sha": b}}
					if strings.Contains(mode, "pipeline") {
						pipelineSHA, pipelineProject := head, 2
						if mode == "wrong_pipeline_sha" {
							pipelineSHA = base
						}
						if mode == "wrong_pipeline_project" {
							pipelineProject = 1
						}
						response["head_pipeline"] = map[string]any{"id": 73, "project_id": pipelineProject, "sha": pipelineSHA}
					}
					json.NewEncoder(w).Encode(response)
					return
				}
				if req.Method == "POST" && req.URL.Path == "/api/v4/projects/2/statuses/"+head {
					posts++
					var payload map[string]any
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if payload["state"] != "skipped" || payload["name"] != "aimangebot/"+strings.Repeat("a", 32)+"/project/1/mr/3" || payload["ref"] != "feature" || strings.Contains(payload["description"].(string), "PRIVATE") {
						t.Error("wrong status payload", payload)
					}
					if mode == "pipeline" && payload["pipeline_id"] != float64(73) {
						t.Error("MR pipeline not selected", payload)
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
			result := repo.PublishRunCheck(context.Background(), run, "https://audit.example/#/runs/9", false, strings.Repeat("a", 32), func(context.Context) error { return nil })
			want := "unknown"
			if mode == "ack" || mode == "pipeline" {
				want = "published"
			}
			if strings.HasPrefix(mode, "stale") || mode == "source_changed" {
				want = "stale"
			}
			if strings.HasPrefix(mode, "wrong_pipeline") {
				want = "failed"
			}
			if result.State != want {
				t.Fatal(result)
			}
			if (want == "stale" || want == "failed") && posts != 0 {
				t.Fatal("stale snapshot published")
			}
			if want != "stale" && want != "failed" && posts != 1 {
				t.Fatal("wrong post count", posts)
			}
			invalid := run
			invalid.AuditPolicy = &AuditPolicy{Workflow: &WorkflowPolicy{Checks: &CheckRules{Mode: "invalid"}}}
			if outcome := repo.PublishRunCheck(context.Background(), invalid, "", false, strings.Repeat("a", 32), func(context.Context) error { return nil }); outcome.Code != "invalid_check_policy" {
				t.Fatal("invalid historical policy published", outcome)
			}
			before := posts
			if repo.PublishRunCheck(context.Background(), run, "https://user:password@example.test", false, strings.Repeat("a", 32), func(context.Context) error { return nil }).State != "failed" || posts != before {
				t.Fatal("unsafe URL published")
			}
		})
	}
}

func TestGitLabCheckAdviceAndBlockingMapping(t *testing.T) {
	rules := CheckRules{Mode: "advisory", MinimumSeverity: "high", BlockOnFailure: true, BlockOnIncomplete: true}
	for _, status := range []string{"failed", "incomplete", "unexpected"} {
		run := Run{ID: 7, Snapshot: Snapshot{HeadSHA: strings.Repeat("a", 40)}, Status: status}
		rules.Mode = "advisory"
		state, _ := checkStateForRules(run, rules)
		if state != "skipped" {
			t.Fatal(state)
		}
		rules.Mode = "blocking"
		state, _ = checkStateForRules(run, rules)
		if state != "failed" {
			t.Fatal(state)
		}
	}
	for status, want := range map[string]string{"succeeded": "success", "pending": "pending", "running": "running", "cancelled": "canceled", "skipped": "skipped"} {
		run := Run{ID: 7, Snapshot: Snapshot{HeadSHA: strings.Repeat("a", 40)}, Status: status}
		state, _ := checkStateForRules(run, rules)
		if state != want {
			t.Fatal(status, state)
		}
	}
}

func TestGitLabCheckNamespacesIsolateInstallations(t *testing.T) {
	head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
	names := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"source_project_id": 1, "diff_refs": map[string]string{"head_sha": head, "base_sha": base}})
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		name, _ := payload["name"].(string)
		names = append(names, name)
		json.NewEncoder(w).Encode(map[string]any{"id": 42, "sha": head, "name": name, "status": payload["state"]})
	}))
	defer server.Close()
	repo, err := NewGitLabRepository("fixture", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	run := Run{ID: 9, Snapshot: Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, HeadSHA: head, BaseSHA: base}, Status: "succeeded"}
	authorize := func(context.Context) error { return nil }
	for _, ns := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32)} {
		if outcome := repo.PublishRunCheck(context.Background(), run, "", false, ns, authorize); outcome.State != "published" {
			t.Fatal(outcome)
		}
	}
	if len(names) != 2 || names[0] == names[1] {
		t.Fatal("installation collision", names)
	}
	if outcome := repo.PublishRunCheck(context.Background(), run, "", false, "invalid", authorize); outcome.State != "failed" || len(names) != 2 {
		t.Fatal("invalid namespace published", outcome)
	}
	// Re-auditing the same commit and MR must use the same job name even
	// when the new local run has a different identity and outcome.
	run.ID++
	run.Status = "failed"
	if outcome := repo.PublishRunCheck(context.Background(), run, "", true, strings.Repeat("a", 32), authorize); outcome.State != "published" {
		t.Fatal(outcome)
	}
	if names[2] != names[0] {
		t.Fatal("re-audit left a separate job", names)
	}
	run.MRIID++
	if outcome := repo.PublishRunCheck(context.Background(), run, "", true, strings.Repeat("a", 32), authorize); outcome.State != "published" {
		t.Fatal(outcome)
	}
	if names[3] == names[0] {
		t.Fatal("different MRs share a job", names)
	}
	run.ProjectID++
	if outcome := repo.PublishRunCheck(context.Background(), run, "", true, strings.Repeat("a", 32), authorize); outcome.State != "published" {
		t.Fatal(outcome)
	}
	if names[4] == names[3] {
		t.Fatal("different target projects share a job", names)
	}
}
