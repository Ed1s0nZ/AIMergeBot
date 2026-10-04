package platform

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/xanzy/go-gitlab"
)

type Change struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Diff    string `json:"diff"`
	Deleted bool   `json:"deleted"`
	Renamed bool   `json:"renamed"`
}

type Repository interface {
	Snapshot(context.Context, int, int) (Snapshot, error)
	Changes(context.Context, Snapshot) ([]Change, []string, error)
	ReadFile(context.Context, Snapshot, string, bool) (string, error)
	ListFiles(context.Context, Snapshot, int) ([]string, bool, error)
}

type GitLabRepository struct {
	Client *gitlab.Client
	Token  string
}

func NewGitLabRepository(token, baseURL string) (*GitLabRepository, error) {
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(strings.TrimRight(baseURL, "/")+"/api/v4"))
	if err != nil {
		return nil, err
	}
	return &GitLabRepository{Client: client, Token: token}, nil
}

func (g *GitLabRepository) Snapshot(ctx context.Context, pid, iid int) (Snapshot, error) {
	mr, _, err := g.Client.MergeRequests.GetMergeRequest(pid, iid, &gitlab.GetMergeRequestsOptions{}, gitlab.WithContext(ctx))
	if err != nil {
		return Snapshot{}, err
	}
	if mr.DiffRefs.HeadSha == "" || mr.DiffRefs.BaseSha == "" {
		return Snapshot{}, fmt.Errorf("merge request has no complete diff refs")
	}
	source := mr.SourceProjectID
	if source <= 0 {
		source = pid
	}
	for page := 1; page <= 20; page++ {
		versions, res, err := g.Client.MergeRequests.GetMergeRequestDiffVersions(pid, iid, &gitlab.GetMergeRequestDiffVersionsOptions{Page: page, PerPage: 100}, gitlab.WithContext(ctx))
		if err != nil {
			return Snapshot{}, err
		}
		for _, v := range versions {
			if v.HeadCommitSHA == mr.DiffRefs.HeadSha && v.BaseCommitSHA == mr.DiffRefs.BaseSha {
				return Snapshot{ProjectID: pid, SourceProjectID: source, MRIID: iid, DiffVersionID: v.ID, BaseSHA: mr.DiffRefs.BaseSha, HeadSHA: mr.DiffRefs.HeadSha, Title: mr.Title, URL: mr.WebURL}, nil
			}
		}
		if res.NextPage == 0 {
			break
		}
	}
	return Snapshot{}, fmt.Errorf("matching diff version unavailable; retry after GitLab finishes preparing diffs")
}

func (g *GitLabRepository) Changes(ctx context.Context, s Snapshot) ([]Change, []string, error) {
	if s.DiffVersionID <= 0 {
		return nil, nil, fmt.Errorf("snapshot lacks pinned diff version; create a new audit")
	}
	var version struct {
		Head     string `json:"head_commit_sha"`
		Base     string `json:"base_commit_sha"`
		RealSize string `json:"real_size"`
		State    string `json:"state"`
		Diffs    []struct {
			OldPath   string `json:"old_path"`
			NewPath   string `json:"new_path"`
			Diff      string `json:"diff"`
			Deleted   bool   `json:"deleted_file"`
			Renamed   bool   `json:"renamed_file"`
			Collapsed bool   `json:"collapsed"`
			TooLarge  bool   `json:"too_large"`
		} `json:"diffs"`
	}
	req, err := g.Client.NewRequest(http.MethodGet, fmt.Sprintf("projects/%d/merge_requests/%d/versions/%d", s.ProjectID, s.MRIID, s.DiffVersionID), nil, []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	if _, err = g.Client.Do(req, &version); err != nil {
		return nil, nil, err
	}
	if version.Head != s.HeadSHA || version.Base != s.BaseSHA {
		return nil, nil, fmt.Errorf("diff version does not match pinned snapshot")
	}
	notes := []string{}
	if version.State != "collected" {
		notes = append(notes, "GitLab diff version is not fully collected: "+version.State)
	}
	count, err := strconv.Atoi(version.RealSize)
	if err != nil || count != len(version.Diffs) {
		notes = append(notes, "GitLab reported a limited or unknown diff file count")
	}
	out := []Change{}
	for _, c := range version.Diffs {
		if c.Collapsed || c.TooLarge {
			notes = append(notes, "GitLab omitted diff content: "+c.NewPath)
		}
		out = append(out, Change{OldPath: c.OldPath, NewPath: c.NewPath, Diff: c.Diff, Deleted: c.Deleted, Renamed: c.Renamed})
	}
	return out, notes, nil
}

func validPath(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsRune(p, '\x00')
}

func (g *GitLabRepository) ReadFile(ctx context.Context, s Snapshot, p string, base bool) (string, error) {
	if !validPath(p) {
		return "", fmt.Errorf("invalid repository path")
	}
	pid, ref := s.SourceProjectID, s.HeadSHA
	if base {
		pid, ref = s.ProjectID, s.BaseSHA
	}
	f, _, err := g.Client.RepositoryFiles.GetFile(pid, p, &gitlab.GetFileOptions{Ref: gitlab.String(ref)}, gitlab.WithContext(ctx))
	if err != nil {
		return "", err
	}
	if f.Size > 256*1024 {
		return "", fmt.Errorf("file exceeds 256 KiB read budget")
	}
	raw, err := base64.StdEncoding.DecodeString(f.Content)
	if err != nil {
		return "", err
	}
	if len(raw) > 256*1024 {
		return "", fmt.Errorf("file exceeds read budget")
	}
	return string(raw), nil
}

func (g *GitLabRepository) ListFiles(ctx context.Context, s Snapshot, pageNo int) ([]string, bool, error) {
	if pageNo < 1 || pageNo > 20 {
		return nil, false, fmt.Errorf("page must be 1–20")
	}
	entries, res, err := g.Client.Repositories.ListTree(s.SourceProjectID, &gitlab.ListTreeOptions{Ref: gitlab.String(s.HeadSHA), Recursive: gitlab.Bool(true), ListOptions: gitlab.ListOptions{Page: pageNo, PerPage: 100}}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, false, err
	}
	out := []string{}
	for _, e := range entries {
		if e.Type == "blob" {
			out = append(out, e.Path)
		}
	}
	return out, res.NextPage > 0, nil
}
