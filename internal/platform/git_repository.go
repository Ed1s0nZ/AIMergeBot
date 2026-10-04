package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/xanzy/go-gitlab"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitRepository reads commit objects without checking out or executing repository code.
type GitRepository struct {
	Directory      string
	Metadata       Repository
	HistoryLimited bool
}

var commitID = regexp.MustCompile(`^[a-fA-F0-9]{40}([a-fA-F0-9]{24})?$`)

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("Git output budget exceeded; narrow the query")
	}
	return b.Buffer.Write(p)
}

func gitCommand(ctx context.Context, dir string, extraEnv []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defaults := []string{"--no-pager", "--literal-pathspecs", "-c", "core.hooksPath=/dev/null", "-c", "core.attributesFile=/dev/null", "-c", "diff.external=", "-c", "protocol.allow=never", "-c", "protocol.https.allow=always", "-c", "protocol.http.allow=always", "-c", "http.followRedirects=false", "-c", "credential.helper=", "-c", "maintenance.auto=false", "-c", "gc.auto=0"}
	cmd := exec.CommandContext(ctx, "git", append(defaults, args...)...)
	cmd.Dir = dir
	// Do not inherit caller Git configuration, tokens, SSH agents or helpers.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "LANG=C.UTF-8", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_NO_REPLACE_OBJECTS=1"}
	cmd.Env = append(cmd.Env, extraEnv...)
	out := &limitedBuffer{limit: 8 * 1024 * 1024}
	stderr := &limitedBuffer{limit: 4096}
	cmd.Stdout = out
	cmd.Stderr = stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && len(args) > 0 && args[0] == "grep" {
			return out.String(), nil
		}
		return "", fmt.Errorf("Git %s failed (output omitted to protect credentials): %w", args[0], err)
	}
	return out.String(), nil
}
func (g *GitRepository) command(ctx context.Context, args ...string) (string, error) {
	return gitCommand(ctx, g.Directory, nil, args...)
}
func (g *GitRepository) ref(s Snapshot, base bool) (string, error) {
	r := s.HeadSHA
	if base {
		r = s.BaseSHA
	}
	if !commitID.MatchString(r) {
		return "", fmt.Errorf("snapshot must contain full commit IDs")
	}
	return r, nil
}
func (g *GitRepository) Snapshot(ctx context.Context, p, i int) (Snapshot, error) {
	if g.Metadata == nil {
		return Snapshot{}, fmt.Errorf("standalone Git requires explicit base/head")
	}
	return g.Metadata.Snapshot(ctx, p, i)
}
func (g *GitRepository) ReadFile(ctx context.Context, s Snapshot, p string, base bool) (string, error) {
	if !validPath(p) {
		return "", fmt.Errorf("invalid repository path")
	}
	r, e := g.ref(s, base)
	if e != nil {
		return "", e
	}
	size, e := g.command(ctx, "cat-file", "-s", r+":"+p)
	if e != nil {
		return "", e
	}
	n, e := strconv.Atoi(strings.TrimSpace(size))
	if e != nil || n > 256*1024 {
		return "", fmt.Errorf("file exceeds 256 KiB read budget")
	}
	content, e := g.command(ctx, "cat-file", "blob", r+":"+p)
	if strings.ContainsRune(content, '\x00') {
		return "", fmt.Errorf("binary file cannot be read as text")
	}
	return content, e
}
func (g *GitRepository) paths(ctx context.Context, s Snapshot, base bool) ([]string, error) {
	r, e := g.ref(s, base)
	if e != nil {
		return nil, e
	}
	out, e := g.command(ctx, "ls-tree", "-r", "-z", "--name-only", r)
	if e != nil {
		return nil, e
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(strings.TrimSuffix(out, "\x00"), "\x00"), nil
}
func (g *GitRepository) ListFiles(ctx context.Context, s Snapshot, page int) ([]string, bool, error) {
	if page < 1 {
		return nil, false, fmt.Errorf("page must be positive")
	}
	files, e := g.paths(ctx, s, false)
	if e != nil {
		return nil, false, e
	}
	start := (page - 1) * 100
	if start >= len(files) {
		return []string{}, false, nil
	}
	end := start + 100
	if end > len(files) {
		end = len(files)
	}
	return files[start:end], end < len(files), nil
}
func (g *GitRepository) Changes(ctx context.Context, s Snapshot) ([]Change, []string, error) {
	if _, e := g.ref(s, false); e != nil {
		return nil, nil, e
	}
	if _, e := g.ref(s, true); e != nil {
		return nil, nil, e
	}
	raw, e := g.command(ctx, "diff", "--name-status", "-z", "--find-renames", s.BaseSHA, s.HeadSHA, "--")
	if e != nil {
		return nil, nil, e
	}
	parts := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	out := []Change{}
	notes := []string{}
	for i := 0; i < len(parts) && parts[i] != ""; {
		status := parts[i]
		i++
		if i >= len(parts) {
			return nil, nil, fmt.Errorf("invalid Git diff listing")
		}
		old := parts[i]
		i++
		p := old
		renamed := strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C")
		if renamed {
			if i >= len(parts) {
				return nil, nil, fmt.Errorf("invalid rename")
			}
			p = parts[i]
			i++
		}
		diff, e := g.command(ctx, "diff", "--no-ext-diff", "--no-textconv", "--unified=3", "--find-renames", s.BaseSHA, s.HeadSHA, "--", old, p)
		if e != nil {
			return nil, nil, e
		}
		idx := strings.Index(diff, "@@ ")
		if idx >= 0 {
			diff = diff[idx:]
		} else {
			notes = append(notes, "No textual hunks: "+p)
			diff = ""
		}
		out = append(out, Change{OldPath: old, NewPath: p, Diff: diff, Deleted: status == "D", Renamed: renamed})
	}
	return out, notes, nil
}

type gitSource struct{ URL, SHA string }

// PrepareGitLab resolves trusted target/source clone URLs before fetching fixed commits.
func PrepareGitLab(ctx context.Context, remote *GitLabRepository, s Snapshot, cfg GitAuditSettings) (*GitRepository, func(), error) {
	origin := remote.Client.BaseURL()
	sources := []gitSource{}
	for _, item := range []struct {
		pid int
		sha string
	}{{s.ProjectID, s.BaseSHA}, {s.SourceProjectID, s.HeadSHA}} {
		project, _, e := remote.Client.Projects.GetProject(item.pid, nil, gitlab.WithContext(ctx))
		if e != nil {
			return nil, nil, fmt.Errorf("cannot resolve Git repository")
		}
		u, e := url.Parse(project.HTTPURLToRepo)
		if e != nil || u.Host != origin.Host || u.Scheme != origin.Scheme {
			return nil, nil, fmt.Errorf("Git clone origin must match configured GitLab")
		}
		sources = append(sources, gitSource{u.String(), item.sha})
	}
	g, cleanup, e := prepareGitSources(ctx, sources, remote.Token, s, cfg)
	if e == nil {
		g.Metadata = remote
	}
	return g, cleanup, e
}

// PrepareRemoteGit is an explicit caller-authorized URL; no hosting-platform API is required.
func PrepareRemoteGit(ctx context.Context, rawURL, token string, s Snapshot, cfg GitAuditSettings) (*GitRepository, func(), error) {
	return prepareGitSources(ctx, []gitSource{{rawURL, s.BaseSHA}, {rawURL, s.HeadSHA}}, token, s, cfg)
}
func prepareGitSources(ctx context.Context, sources []gitSource, token string, s Snapshot, cfg GitAuditSettings) (*GitRepository, func(), error) {
	defaultGitAudit(&cfg)
	dir, e := os.MkdirTemp("", "aimangebot-git-*")
	if e != nil {
		return nil, nil, e
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	fail := func(err error) (*GitRepository, func(), error) { cleanup(); return nil, nil, err }
	if !commitID.MatchString(s.BaseSHA) || !commitID.MatchString(s.HeadSHA) {
		return fail(fmt.Errorf("invalid snapshot commit IDs"))
	}
	if _, e = gitCommand(ctx, dir, nil, "init", "--bare", "--template=", dir); e != nil {
		return fail(e)
	}
	for _, item := range sources {
		u, err := url.Parse(item.URL)
		if err != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return fail(fmt.Errorf("Git URL must be HTTP(S), without credentials, query or fragment"))
		}
		env := []string{}
		if token != "" {
			auth := base64.StdEncoding.EncodeToString([]byte("oauth2:" + token))
			env = []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http." + u.Scheme + "://" + u.Host + "/.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Basic " + auth}
		}
		fetchCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		over := make(chan struct{}, 1)
		go func() {
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-done:
					return
				case <-tick.C:
					var total int64
					_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
						if err == nil && !info.IsDir() {
							total += info.Size()
						}
						return nil
					})
					if total > int64(cfg.MaxPackMiB)*1024*1024 {
						select {
						case over <- struct{}{}:
						default:
						}
						cancel()
						return
					}
				}
			}
		}()
		_, err = gitCommand(fetchCtx, dir, env, "fetch", "--no-tags", "--no-recurse-submodules", "--depth="+strconv.Itoa(cfg.HistoryDepth), u.String(), item.SHA)
		close(done)
		cancel()
		select {
		case <-over:
			return fail(fmt.Errorf("Git workspace size budget exceeded"))
		default:
		}
		if err != nil {
			return fail(err)
		}
		var total int64
		_ = filepath.Walk(dir, func(_ string, info os.FileInfo, e error) error {
			if e == nil && !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		if total > int64(cfg.MaxPackMiB)*1024*1024 {
			return fail(fmt.Errorf("Git workspace size budget exceeded"))
		}
	}
	g := &GitRepository{Directory: dir}
	shallow, e := g.command(ctx, "rev-parse", "--is-shallow-repository")
	if e != nil {
		return fail(e)
	}
	g.HistoryLimited = strings.TrimSpace(shallow) == "true"
	for _, sha := range []string{s.BaseSHA, s.HeadSHA} {
		if _, e = g.command(ctx, "cat-file", "-e", sha+"^{commit}"); e != nil {
			return fail(e)
		}
	}
	return g, cleanup, nil
}

// Keep compile-time assurance that output budget writes implement io.Writer.
var _ io.Writer = (*limitedBuffer)(nil)
