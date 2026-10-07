package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
)

type OwnerCandidate struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Source   string `json:"source"`
	Alias    string `json:"alias,omitempty"`
	Line     int    `json:"line,omitempty"`
	Section  string `json:"section,omitempty"`
}
type OwnerRecommendation struct {
	Truncated   bool              `json:"truncated"`
	RunID       int64             `json:"run_id"`
	FindingID   string            `json:"finding_id"`
	Revision    int64             `json:"revision"`
	File        string            `json:"file"`
	Document    CodeOwnerDocument `json:"document"`
	Matches     CodeOwnerMatches  `json:"matches"`
	Candidates  []OwnerCandidate  `json:"candidates"`
	Unmapped    []string          `json:"unmapped"`
	Filtered    bool              `json:"filtered"`
	SourceState string            `json:"source_state"`
	Notes       []string          `json:"notes"`
}
type ownerRecommendationInput struct {
	Snapshot Snapshot
	Policy   OwnerRouting
	File     string
}

func readOwnerRecommendationInput(ctx context.Context, tx *sql.Tx, run, actor int64, finding string) (ownerRecommendationInput, error) {
	input := ownerRecommendationInput{}
	var policy string
	if err := tx.QueryRowContext(ctx, `SELECT project_id,source_project_id,base_sha,head_sha,mr_iid,CASE WHEN length(CAST(audit_policy_json AS BLOB))<=65536 THEN audit_policy_json ELSE '' END FROM platform_runs WHERE id=?`, run).Scan(&input.Snapshot.ProjectID, &input.Snapshot.SourceProjectID, &input.Snapshot.BaseSHA, &input.Snapshot.HeadSHA, &input.Snapshot.MRIID, &policy); err != nil {
		return input, err
	}
	if _, err := requireSnapshotRole(ctx, tx, input.Snapshot, actor, "viewer"); err != nil {
		return input, err
	}
	if err := json.Unmarshal([]byte(policy), &input.Snapshot.AuditPolicy); err != nil {
		return input, ErrConflict
	}
	if _, err := requireSnapshotRole(ctx, tx, input.Snapshot, actor, "viewer"); err != nil {
		return input, err
	}
	if err := requireTicketProjectsEnabled(ctx, tx, input.Snapshot); err != nil {
		return input, err
	}
	if err := requireLegacyRepositorySnapshot(ctx, tx, input.Snapshot); err != nil {
		return input, err
	}
	if !validCodeOwnerSHA(input.Snapshot.HeadSHA) {
		return input, ErrConflict
	}
	var result string
	if err := tx.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(result_json AS BLOB))<=1048576 THEN result_json ELSE '' END FROM platform_runs WHERE id=?`, run).Scan(&result); err != nil {
		return input, err
	}
	var report AuditResult
	if err := json.Unmarshal([]byte(result), &report); err != nil {
		return input, ErrConflict
	}
	count := 0
	for _, item := range report.Findings {
		if item.ID == finding {
			count++
			input.File = item.File
		}
	}
	if count == 0 {
		return input, sql.ErrNoRows
	}
	if count != 1 || !validPath(input.File) || len(input.File) > 4096 {
		return input, ErrConflict
	}
	p, err := readOwnerRouting(ctx, tx, input.Snapshot.ProjectID)
	if err != nil {
		return input, err
	}
	if _, err := validateOwnerRouting(p); err != nil {
		return input, ErrConflict
	}
	input.Policy = p
	return input, nil
}

// External repository reads occur without a database transaction. Snapshot,
// policy and current viewer/candidate permissions are rechecked afterwards.
func (s *Store) RecommendFindingOwners(ctx context.Context, run, actor int64, finding string, repo Repository) (OwnerRecommendation, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return OwnerRecommendation{}, err
	}
	input, err := readOwnerRecommendationInput(ctx, tx, run, actor, finding)
	if err != nil {
		tx.Rollback()
		return OwnerRecommendation{}, err
	}
	if err := tx.Commit(); err != nil {
		return OwnerRecommendation{}, err
	}
	out := OwnerRecommendation{RunID: run, FindingID: finding, Revision: input.Policy.Revision, File: input.File, Candidates: []OwnerCandidate{}, Unmapped: []string{}, Notes: []string{}, Matches: CodeOwnerMatches{Rules: []CodeOwnerRule{}, Exclusions: []CodeOwnerRule{}}, SourceState: "unavailable"}
	doc, readErr := LoadCodeOwnerDocument(ctx, repo, input.Snapshot)
	out.Document = doc
	if readErr != nil {
		out.Notes = append(out.Notes, "CODEOWNERS 来源无法完整读取或解析；不能据此认定文件没有责任人。")
	} else if doc.Present {
		out.SourceState = "available"
		out.Matches, readErr = doc.Rules.Match(input.File)
		if readErr != nil {
			out.SourceState = "unavailable"
			out.Notes = append(out.Notes, "路径无法按当前 CODEOWNERS 规则匹配。")
		}
	} else {
		out.SourceState = "missing"
	}
	if ctx.Err() != nil {
		return OwnerRecommendation{}, ctx.Err()
	}
	tx, err = s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return OwnerRecommendation{}, err
	}
	defer tx.Rollback()
	current, err := readOwnerRecommendationInput(ctx, tx, run, actor, finding)
	if err != nil {
		return OwnerRecommendation{}, err
	}
	if !reflect.DeepEqual(input, current) {
		return OwnerRecommendation{}, ErrConflict
	}
	type eligibility struct {
		allowed  bool
		username string
	}
	eligible := map[int64]eligibility{}
	add := func(id int64, source, alias string, line int, section string) error {
		cached, known := eligible[id]
		if !known {
			if _, err := requireSnapshotRole(ctx, tx, input.Snapshot, id, "viewer"); err != nil {
				if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrCredentials) || errors.Is(err, ErrProjectPermission) {
					eligible[id] = eligibility{}
					out.Filtered = true
					return nil
				}
				return err
			}
			if err := tx.QueryRowContext(ctx, `SELECT username FROM platform_users WHERE id=? AND disabled=0`, id).Scan(&cached.username); err != nil {
				return err
			}
			cached.allowed = true
			eligible[id] = cached
		}
		if !cached.allowed {
			out.Filtered = true
			return nil
		}
		if len(out.Candidates) >= 200 {
			out.Truncated = true
			return nil
		}
		out.Candidates = append(out.Candidates, OwnerCandidate{UserID: id, Username: cached.username, Source: source, Alias: alias, Line: line, Section: section})
		return nil
	}
	hasRule := len(out.Matches.Rules) > 0 || len(out.Matches.Exclusions) > 0
	if out.SourceState == "available" && hasRule {
		seen := map[string]bool{}
		for _, rule := range out.Matches.Rules {
			for _, alias := range rule.Owners {
				ids := input.Policy.Aliases[alias]
				if len(ids) == 0 {
					if !seen[alias] {
						seen[alias] = true
						if len(out.Unmapped) < 100 {
							out.Unmapped = append(out.Unmapped, alias)
						} else {
							out.Truncated = true
						}
					}
					continue
				}
				for _, id := range ids {
					if err := add(id, "codeowners", alias, rule.Line, rule.Section); err != nil {
						return OwnerRecommendation{}, err
					}
				}
			}
		}
	} else if input.Policy.DefaultOwner > 0 {
		if err := add(input.Policy.DefaultOwner, "project_default", "", 0, ""); err != nil {
			return OwnerRecommendation{}, err
		}
	}
	if len(out.Matches.Rules) > 200 {
		out.Matches.Rules = out.Matches.Rules[:200]
		out.Truncated = true
	}
	if len(out.Matches.Exclusions) > 200 {
		out.Matches.Exclusions = out.Matches.Exclusions[:200]
		out.Truncated = true
	}
	if out.Truncated {
		out.Notes = append(out.Notes, "推荐候选/匹配依据仅展示前 200 条，未映射身份仅展示前 100 条；本结果不是完整责任人名单。")
	}
	sort.Strings(out.Unmapped)
	if out.Filtered {
		out.Notes = append(out.Notes, "部分候选账号已停用或缺少本次审计完整仓库范围的权限，已过滤。")
	}
	out.Notes = append(out.Notes, "推荐仅作责任分配参考，不授予权限、不修改已有责任人，也不代表代码平台必审人。")
	if err := tx.Commit(); err != nil {
		return OwnerRecommendation{}, err
	}
	return out, nil
}
