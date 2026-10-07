package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func ownerRecommendationFixture(t *testing.T) (*Store, User, User, int64, Snapshot, *codeOwnerSourceFixture) {
	t.Helper()
	s, admin, viewer := accessFixture(t)
	ctx := context.Background()
	if err := s.SaveProject(ctx, Project{ID: 3, Name: "source", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, project := range []int{1, 2, 3} {
		if err := s.SetProjectMember(ctx, project, viewer.ID, "viewer", admin.ID); err != nil {
			t.Fatal(err)
		}
	}
	links := []ContextRepository{{ProjectID: 2, SHA: strings.Repeat("c", 40)}}
	if err := s.SaveContextRepositories(ctx, 1, links, admin.ID); err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 3, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), AuditPolicy: &AuditPolicy{ContextRepositories: links}}
	run, _, err := s.Enqueue(ctx, snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(AuditResult{Findings: []Finding{{ID: "f", File: "source.any", Severity: "high"}}})
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json=? WHERE id=?`, string(result), run); err != nil {
		t.Fatal(err)
	}
	repo := &codeOwnerSourceFixture{provider: "gitlab", snap: snap, dirs: map[string][]CodeOwnerEntry{"": {{"CODEOWNERS", "100644", "blob"}}}, content: map[string]string{"CODEOWNERS": "* @owner @unknown"}}
	return s, admin, viewer, run, snap, repo
}
func TestOwnerRecommendationMapsExplicitAliasesWithFullSnapshotACL(t *testing.T) {
	s, admin, viewer, run, _, repo := ownerRecommendationFixture(t)
	ctx := context.Background()
	owner, err := s.CreateUser(ctx, "owner", "owner-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range []int{1, 3} {
		if err := s.SetProjectMember(ctx, project, owner.ID, "viewer", admin.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, OwnerRouting{DefaultOwner: admin.ID, Aliases: map[string][]int64{"@owner": {owner.ID}}}); err != nil {
		t.Fatal(err)
	}
	out, err := s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo)
	if err != nil || len(out.Candidates) != 0 || !out.Filtered || len(out.Unmapped) != 1 || out.Unmapped[0] != "@unknown" || out.Document.RepositoryID != 3 {
		t.Fatal(out, err)
	}
	if err := s.SetProjectMember(ctx, 2, owner.ID, "viewer", admin.ID); err != nil {
		t.Fatal(err)
	}
	out, err = s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo)
	if err != nil || len(out.Candidates) != 1 || out.Candidates[0].UserID != owner.ID || out.Candidates[0].Username != "owner" || out.Candidates[0].Source != "codeowners" || out.Candidates[0].Line != 1 || out.Filtered {
		t.Fatal(out, err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_users SET disabled=1 WHERE id=?`, owner.ID); err != nil {
		t.Fatal(err)
	}
	out, err = s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo)
	if err != nil || len(out.Candidates) != 0 || !out.Filtered {
		t.Fatal("disabled owner retained", out, err)
	}
}
func TestOwnerRecommendationDefaultMissingUnmappedAndExplicitExclusion(t *testing.T) {
	s, admin, viewer, run, _, repo := ownerRecommendationFixture(t)
	ctx := context.Background()
	if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, OwnerRouting{DefaultOwner: admin.ID}); err != nil {
		t.Fatal(err)
	}
	out, err := s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo)
	if err != nil || len(out.Candidates) != 0 || len(out.Unmapped) != 2 {
		t.Fatal("unmapped fell back to default", out, err)
	}
	repo.content["CODEOWNERS"] = "* @owner\n!*.any"
	out, err = s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo)
	if err != nil || len(out.Candidates) != 0 || len(out.Matches.Exclusions) != 1 {
		t.Fatal("exclusion fell back to default", out, err)
	}
	repo.dirs[""] = nil
	out, err = s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo)
	if err != nil || out.SourceState != "missing" || len(out.Candidates) != 1 || out.Candidates[0].Source != "project_default" {
		t.Fatal(out, err)
	}
	out, err = s.RecommendFindingOwners(ctx, run, viewer.ID, "f", nil)
	if err != nil || out.SourceState != "unavailable" || len(out.Candidates) != 1 || len(out.Notes) < 2 {
		t.Fatal("unavailable disguised as missing", out, err)
	}
}

type ownerChangingRepo struct {
	*codeOwnerSourceFixture
	change func()
}

func (r *ownerChangingRepo) ReadFile(ctx context.Context, s Snapshot, file string, base bool) (string, error) {
	content, err := r.codeOwnerSourceFixture.ReadFile(ctx, s, file, base)
	r.change()
	return content, err
}
func TestOwnerRecommendationRechecksPolicyViewerAndCandidateAfterSourceRead(t *testing.T) {
	for _, scenario := range []string{"viewer target", "viewer source", "viewer context", "policy", "candidate"} {
		t.Run(scenario, func(t *testing.T) {
			s, admin, viewer, run, _, repo := ownerRecommendationFixture(t)
			ctx := context.Background()
			owner, err := s.CreateUser(ctx, "owner", "owner-long-password", "member")
			if err != nil {
				t.Fatal(err)
			}
			for _, project := range []int{1, 2, 3} {
				if err := s.SetProjectMember(ctx, project, owner.ID, "viewer", admin.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.SaveOwnerRouting(ctx, 1, admin.ID, 0, OwnerRouting{Aliases: map[string][]int64{"@owner": {owner.ID}}}); err != nil {
				t.Fatal(err)
			}
			changing := &ownerChangingRepo{codeOwnerSourceFixture: repo, change: func() {
				switch scenario {
				case "policy":
					_, err = s.SaveOwnerRouting(ctx, 1, admin.ID, 1, OwnerRouting{})
				case "candidate":
					err = s.SetProjectMember(ctx, 2, owner.ID, "", admin.ID)
				default:
					project := 1
					if scenario == "viewer source" {
						project = 3
					}
					if scenario == "viewer context" {
						project = 2
					}
					err = s.SetProjectMember(ctx, project, viewer.ID, "", admin.ID)
				}
				if err != nil {
					t.Fatal(err)
				}
			}}
			out, err := s.RecommendFindingOwners(ctx, run, viewer.ID, "f", changing)
			if scenario == "candidate" {
				if err != nil || len(out.Candidates) != 0 || !out.Filtered {
					t.Fatal(out, err)
				}
			} else if scenario == "policy" {
				if !errors.Is(err, ErrConflict) {
					t.Fatal(out, err)
				}
			} else if err == nil {
				t.Fatal("viewer revocation disclosed recommendation", out)
			}
		})
	}
}
func TestOwnerRecommendationDeniesBeforeRepositoryRead(t *testing.T) {
	for _, project := range []int{1, 2, 3} {
		s, admin, viewer, run, _, repo := ownerRecommendationFixture(t)
		if err := s.SetProjectMember(context.Background(), project, viewer.ID, "", admin.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.RecommendFindingOwners(context.Background(), run, viewer.ID, "f", repo); err == nil || len(repo.reads) != 0 {
			t.Fatal("unauthorized source read", project, repo.reads, err)
		}
	}
}

func TestOwnerRecommendationBoundsCandidatesAndMatchingEvidence(t *testing.T) {
	s, admin, viewer, run, _, repo := ownerRecommendationFixture(t)
	if _, err := s.SaveOwnerRouting(context.Background(), 1, admin.ID, 0, OwnerRouting{Aliases: map[string][]int64{"@owner": {admin.ID}}}); err != nil {
		t.Fatal(err)
	}
	var raw strings.Builder
	for i := 0; i < 201; i++ {
		fmt.Fprintf(&raw, "[section%d]\n* @owner\n", i)
	}
	repo.content["CODEOWNERS"] = raw.String()
	out, err := s.RecommendFindingOwners(context.Background(), run, viewer.ID, "f", repo)
	if err != nil || !out.Truncated || len(out.Candidates) != 200 || len(out.Matches.Rules) != 200 || len(out.Notes) < 2 {
		t.Fatal(out, err)
	}
}

func TestOwnerRecommendationRejectsAmbiguousFindingAndDisabledProject(t *testing.T) {
	s, _, viewer, run, _, repo := ownerRecommendationFixture(t)
	ctx := context.Background()
	raw := `{"findings":[{"id":"f","file":"one.any"},{"id":"f","file":"two.any"}]}`
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json=? WHERE id=?`, raw, run); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo); !errors.Is(err, ErrConflict) || len(repo.reads) != 0 {
		t.Fatal("ambiguous finding used", err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"f","file":"source.any"}]}' WHERE id=?`, run); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE platform_projects SET enabled=0 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecommendFindingOwners(ctx, run, viewer.ID, "f", repo); !errors.Is(err, ErrProjectPermission) || len(repo.reads) != 0 {
		t.Fatal("disabled context used", err)
	}
}
