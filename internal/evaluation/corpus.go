package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Anchor struct {
	File string `json:"file"`
	Side string `json:"side"`
	Line int    `json:"line"`
}
type Case struct {
	ID             string            `json:"id"`
	Expectation    string            `json:"expectation"`
	BaseFiles      map[string]string `json:"base_files"`
	HeadFiles      map[string]string `json:"head_files"`
	ExpectedAnchor Anchor            `json:"expected_anchor"`
	Rationale      string            `json:"rationale"`
}
type Corpus struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Cases   []Case `json:"cases"`
}

func LoadCorpus(path string) (Corpus, string, error) {
	raw, err := os.ReadFile(path)
	var corpus Corpus
	if err != nil {
		return corpus, "", err
	}
	if len(raw) > 1024*1024 {
		return corpus, "", fmt.Errorf("corpus exceeds budget")
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		return corpus, "", err
	}
	if corpus.Version != 1 || len(corpus.Cases) == 0 || len(corpus.Cases) > 100 {
		return corpus, "", fmt.Errorf("unsupported corpus")
	}
	seen := map[string]bool{}
	idPattern := regexp.MustCompile(`^case-[0-9]{3}$`)
	for _, c := range corpus.Cases {
		if !idPattern.MatchString(c.ID) || seen[c.ID] {
			return corpus, "", fmt.Errorf("invalid or duplicate neutral case ID")
		}
		seen[c.ID] = true
		if c.Expectation != "positive" && c.Expectation != "negative" && c.Expectation != "uncertain" {
			return corpus, "", fmt.Errorf("invalid expectation")
		}
		for _, files := range []map[string]string{c.BaseFiles, c.HeadFiles} {
			if len(files) == 0 || len(files) > 100 {
				return corpus, "", fmt.Errorf("invalid fixture files")
			}
			for p, content := range files {
				if !safeFixturePath(p) || len(content) > 256*1024 {
					return corpus, "", fmt.Errorf("unsafe fixture path or budget")
				}
			}
		}
		files := c.HeadFiles
		if c.ExpectedAnchor.Side == "base" {
			files = c.BaseFiles
		} else if c.ExpectedAnchor.Side != "head" {
			return corpus, "", fmt.Errorf("invalid anchor side")
		}
		text, ok := files[c.ExpectedAnchor.File]
		if !ok || c.ExpectedAnchor.Line < 1 || c.ExpectedAnchor.Line > len(strings.Split(strings.TrimSuffix(text, "\n"), "\n")) || c.Rationale == "" {
			return corpus, "", fmt.Errorf("invalid ground truth anchor")
		}
	}
	digest := sha256.Sum256(raw)
	return corpus, hex.EncodeToString(digest[:]), nil
}

func BuildRepository(ctx context.Context, dir string, c Case) (string, string, error) {
	for _, files := range []map[string]string{c.BaseFiles, c.HeadFiles} {
		for p := range files {
			if !safeFixturePath(p) {
				return "", "", fmt.Errorf("unsafe fixture path")
			}
		}
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", "", err
	}
	git := func(args ...string) (string, error) {
		prefix := []string{"--no-pager", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "credential.helper="}
		cmd := exec.CommandContext(ctx, "git", append(prefix, args...)...)
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "LANG=C.UTF-8", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z"}
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("fixture Git operation failed")
		}
		return strings.TrimSpace(string(out)), nil
	}
	if _, err := git("init", "--quiet", "--initial-branch=main"); err != nil {
		return "", "", err
	}
	write := func(files map[string]string) error {
		paths := []string{}
		for p := range files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			target := filepath.Join(dir, p)
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte(files[p]), 0600); err != nil {
				return err
			}
		}
		return nil
	}
	if err := write(c.BaseFiles); err != nil {
		return "", "", err
	}
	if _, err := git("add", "--all"); err != nil {
		return "", "", err
	}
	if _, err := git("commit", "--quiet", "-m", "base"); err != nil {
		return "", "", err
	}
	base, err := git("rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	for p := range c.BaseFiles {
		if _, ok := c.HeadFiles[p]; !ok {
			if err = os.Remove(filepath.Join(dir, p)); err != nil {
				return "", "", err
			}
		}
	}
	if err = write(c.HeadFiles); err != nil {
		return "", "", err
	}
	if _, err = git("add", "--all"); err != nil {
		return "", "", err
	}
	if _, err = git("commit", "--quiet", "-m", "head"); err != nil {
		return "", "", err
	}
	head, err := git("rev-parse", "HEAD")
	return base, head, err
}

func safeFixturePath(p string) bool {
	if filepath.IsAbs(p) || filepath.Clean(p) != p || p == "." || p == ".." || strings.HasPrefix(p, "../") || strings.ContainsAny(p, "\\\x00") || len(p) > 1024 {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}
