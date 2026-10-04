package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitLabForkSnapshotAndMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v4/projects/1/merge_requests/2":
			fmt.Fprint(w, `{"source_project_id":7,"title":"fork","diff_refs":{"head_sha":"head","base_sha":"base"}}`)
		case r.URL.Path == "/api/v4/projects/1/merge_requests/2/versions":
			fmt.Fprint(w, `[{"id":5,"head_commit_sha":"head","base_commit_sha":"base"}]`)
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/7/repository/files/"):
			if r.URL.Query().Get("ref") != "head" {
				t.Error("head ref mismatch")
			}
			fmt.Fprintf(w, `{"content":%q,"size":4}`, base64.StdEncoding.EncodeToString([]byte("head")))
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/1/repository/files/"):
			if r.URL.Query().Get("ref") != "base" {
				t.Error("base ref mismatch")
			}
			fmt.Fprintf(w, `{"content":%q,"size":4}`, base64.StdEncoding.EncodeToString([]byte("base")))
		case r.URL.Path == "/api/v4/projects/1/merge_requests/2/versions/5":
			fmt.Fprint(w, `{"head_commit_sha":"updated","base_commit_sha":"base","diffs":[]}`)
		default:
			t.Errorf("unexpected GitLab path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repo, err := NewGitLabRepository("fixture", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	s, err := repo.Snapshot(context.Background(), 1, 2)
	if err != nil || s.SourceProjectID != 7 {
		t.Fatal("fork snapshot incorrect", err)
	}
	head, err := repo.ReadFile(context.Background(), s, "a.go", false)
	if err != nil || head != "head" {
		t.Fatal("head wrong", err)
	}
	base, err := repo.ReadFile(context.Background(), s, "a.go", true)
	if err != nil || base != "base" {
		t.Fatal("base wrong", err)
	}
	if _, _, err = repo.Changes(context.Background(), s); err == nil {
		t.Fatal("changed MR silently audited")
	}
	if _, err = repo.ReadFile(context.Background(), s, "../config.yaml", false); err == nil {
		t.Fatal("traversal accepted")
	}
}
func TestLegacyImportIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(`CREATE TABLE results(id INTEGER PRIMARY KEY,result_json TEXT);INSERT INTO results VALUES(1,'{"project_id":1,"mr_id":2,"project_name":"legacy","result":[{"type":"XSS","level":"medium","file":"a.go","code":"old"}]}')`); err != nil {
		t.Fatal(err)
	}
	if err := s.ImportLegacy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ImportLegacy(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_runs`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate legacy import")
	}
	r, err := s.Run(ctx, 1)
	if err != nil || r.Status != "incomplete" || r.HeadSHA != "" || len(r.Result.Findings) != 1 {
		t.Fatal("legacy evidence invented", err)
	}
}

func TestPinnedDiffVersionPaginationAndCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v4/projects/1/merge_requests/2":
			fmt.Fprint(w, `{"source_project_id":1,"diff_refs":{"head_sha":"head","base_sha":"base"}}`)
		case "/api/v4/projects/1/merge_requests/2/versions":
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("X-Next-Page", "2")
				fmt.Fprint(w, `[{"id":4,"head_commit_sha":"old","base_commit_sha":"base"}]`)
			} else {
				fmt.Fprint(w, `[{"id":5,"head_commit_sha":"head","base_commit_sha":"base"}]`)
			}
		case "/api/v4/projects/1/merge_requests/2/versions/5":
			fmt.Fprint(w, `{"head_commit_sha":"head","base_commit_sha":"base","state":"collected","real_size":"2","diffs":[{"old_path":"a.go","new_path":"a.go","diff":"@@ -0,0 +1 @@\n+new"},{"old_path":"large.go","new_path":"large.go","too_large":true,"diff":""}]}`)
		case "/api/v4/projects/1/repository/tree":
			object := strings.Repeat("a", 40)
			if r.URL.Query().Get("ref") == "head" {
				object = strings.Repeat("b", 40)
			}
			json.NewEncoder(w).Encode([]map[string]string{{"path": "a.go", "mode": "100644", "type": "blob", "id": object}, {"path": "large.go", "mode": "100644", "type": "blob", "id": object}})

		default:
			t.Errorf("unbound diff request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repo, err := NewGitLabRepository("fixture", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	s, err := repo.Snapshot(context.Background(), 1, 2)
	if err != nil || s.DiffVersionID != 5 {
		t.Fatal("diff version pagination failed", err)
	}
	changes, notes, err := repo.Changes(context.Background(), s)
	if err != nil || len(changes) != 2 || len(notes) != 0 || len(changes[1].Notes) != 1 {
		t.Fatal("large diff omitted silently", err, notes)
	}
}
