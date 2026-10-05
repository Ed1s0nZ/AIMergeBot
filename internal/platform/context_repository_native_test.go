package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextNativePreparationPinsObjectsAndCleansWorkspace(t *testing.T) {
	s, admin, _, snap := contextFixture(t)
	g, fixed := localGitFixture(t)
	ctx := context.Background()
	sha := fixed.BaseSHA // The remote HEAD differs from this selected version.
	if err := s.SaveContextRepositories(ctx, 1, []ContextRepository{{ProjectID: 2, SHA: sha}}, admin.ID); err != nil {
		t.Fatal(err)
	}
	snap.AuditPolicy.ContextRepositories[0].SHA = sha
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/2/repository/commits/" + sha:
			json.NewEncoder(w).Encode(map[string]string{"id": sha})
			return
		case "/api/v4/projects/2":
			json.NewEncoder(w).Encode(map[string]any{"id": 2, "http_url_to_repo": baseURL + "/repo.git"})
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/repo.git/") {
			http.NotFound(w, r)
			return
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "oauth2" || password != "synthetic-native-token" {
			t.Error("missing scoped fetch authentication")
		}
		cmd := exec.CommandContext(r.Context(), "git", "http-backend")
		cmd.Env = append(os.Environ(), "GIT_PROJECT_ROOT="+filepath.Dir(g.Directory), "GIT_HTTP_EXPORT_ALL=1", "PATH_INFO=/"+filepath.Base(g.Directory)+"/.git/"+strings.TrimPrefix(r.URL.Path, "/repo.git/"), "REQUEST_METHOD="+r.Method, "QUERY_STRING="+r.URL.RawQuery, "CONTENT_TYPE="+r.Header.Get("Content-Type"), fmt.Sprintf("CONTENT_LENGTH=%d", r.ContentLength))
		cmd.Stdin = r.Body
		raw, err := cmd.Output()
		if err != nil {
			t.Error("fixture smart HTTP failed", err)
			w.WriteHeader(500)
			return
		}
		headers, body, ok := strings.Cut(string(raw), "\r\n\r\n")
		if !ok {
			headers, body, ok = strings.Cut(string(raw), "\n\n")
		}
		if !ok {
			t.Error("invalid CGI response")
			w.WriteHeader(500)
			return
		}
		for _, line := range strings.Split(headers, "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok {
				w.Header().Set(k, strings.TrimSpace(v))
			}
		}
		w.Write([]byte(body))
	}))
	defer server.Close()
	baseURL = server.URL
	settings, err := OpenSettings(filepath.Join(t.TempDir(), "config.yaml"), "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := settings.Snapshot()
	cfg.GitLab.URL, cfg.GitLab.Token = baseURL, "synthetic-native-token"
	cfg.GitAudit.Enabled = true
	cfg.GitAudit.MaxPackMiB = 16
	if err = settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	snap.AuditPolicy.RepositoryURL = baseURL
	id, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Claim(ctx)
	run, err := s.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Store: s, Settings: settings}
	sources, notes, cleanup, err := runner.prepareContextSources(ctx, run, cfg.GitAudit, nil)
	defer cleanup()
	if err != nil || len(notes) != 0 {
		t.Fatal("native context preparation", err, notes)
	}
	local, ok := sources[2].Repository.(*GitRepository)
	if !ok {
		t.Fatal("native source not prepared")
	}
	out, _ := crossTools(runRepo{}, run.Snapshot, sources).contextFile(ctx, contextReadArgs{RepositoryID: 2, Path: "guard.any"})
	if out.Error != "" || out.HeadSHA != sha || !strings.Contains(out.Text, "authorize") {
		t.Fatal("read followed moving HEAD", out.Error)
	}
	config, err := os.ReadFile(filepath.Join(local.Directory, "config"))
	if err != nil || strings.Contains(string(config), cfg.GitLab.Token) || strings.Contains(string(config), "Authorization") {
		t.Fatal("fetch credential persisted", err)
	}
	cleanup()
	if _, err = os.Stat(local.Directory); !os.IsNotExist(err) {
		t.Fatal("native context workspace leaked", err)
	}
}
