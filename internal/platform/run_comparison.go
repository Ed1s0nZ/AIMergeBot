package platform

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

var ErrComparisonScope = errors.New("comparison requires two different runs of the same project, source and MR")

type ComparisonIdentity struct {
	ID        int64  `json:"id"`
	ProjectID int    `json:"project_id"`
	MRIID     int    `json:"mr_iid"`
	BaseSHA   string `json:"base_sha"`
	HeadSHA   string `json:"head_sha"`
	Status    string `json:"status"`
}
type ComparisonItem struct {
	State   string   `json:"state"`
	Current *Finding `json:"current,omitempty"`
	Prior   *Finding `json:"prior,omitempty"`
	Changed []string `json:"changed"`
}
type RunComparison struct {
	Current      ComparisonIdentity `json:"current"`
	Prior        ComparisonIdentity `json:"prior"`
	ScopeChanged bool               `json:"scope_changed"`
	Items        []ComparisonItem   `json:"items"`
	Limitations  []string           `json:"limitations"`
	Truncated    bool               `json:"truncated"`
}

func comparisonIdentity(r Run) ComparisonIdentity {
	return ComparisonIdentity{r.ID, r.ProjectID, r.MRIID, r.BaseSHA, r.HeadSHA, r.Status}
}
func compareRunFindings(current, prior Run) RunComparison {
	out := RunComparison{Current: comparisonIdentity(current), Prior: comparisonIdentity(prior), Items: []ComparisonItem{}, Limitations: []string{"未再次观察不表示已修复；源指纹关联不是语义等价证明，人工复核不自动继承。"}}
	a, _ := json.Marshal(current.AuditPolicy)
	b, _ := json.Marshal(prior.AuditPolicy)
	out.ScopeChanged = string(a) != string(b) || current.BaseSHA != prior.BaseSHA
	if out.ScopeChanged {
		out.Limitations = append(out.Limitations, "审计策略或 BASE 不同，结果差异可能由范围变化引起。")
	}
	if current.Status != "succeeded" || prior.Status != "succeeded" {
		out.Limitations = append(out.Limitations, "至少一次运行未完整成功；不能依据缺失发现判断修复。")
	}
	keys := func(r Run) map[string][]int {
		m := map[string][]int{}
		for i, f := range r.Result.Findings {
			fp := findingFingerprint(r.Snapshot, f)
			if fp != "" && f.Fingerprint == fp {
				m[fp] = append(m[fp], i)
			}
		}
		return m
	}
	ck, pk := keys(current), keys(prior)
	matched := map[int]bool{}
	add := func(item ComparisonItem) {
		if len(out.Items) >= 200 {
			out.Truncated = true
			return
		}
		out.Items = append(out.Items, item)
	}
	for i := range current.Result.Findings {
		f := current.Result.Findings[i]
		item := ComparisonItem{State: "newly_observed", Current: &f, Changed: []string{}}
		fp := findingFingerprint(current.Snapshot, f)
		if fp != "" && f.Fingerprint == fp && len(ck[fp]) == 1 && len(pk[fp]) == 1 {
			j := pk[fp][0]
			old := prior.Result.Findings[j]
			matched[j] = true
			item.State = "reobserved"
			item.Prior = &old
			if f.Severity != old.Severity {
				item.Changed = append(item.Changed, "severity")
			}
			if f.Line != old.Line {
				item.Changed = append(item.Changed, "line")
			}
			if f.Description != old.Description {
				item.Changed = append(item.Changed, "description")
			}
			if f.Suggestion != old.Suggestion {
				item.Changed = append(item.Changed, "suggestion")
			}
			if f.Confidence != old.Confidence {
				item.Changed = append(item.Changed, "confidence")
			}
		} else if fp == "" || f.Fingerprint != fp || len(ck[fp]) > 1 || len(pk[fp]) > 1 {
			item.State = "unmatched"
		}
		add(item)
	}
	for i := range prior.Result.Findings {
		if matched[i] {
			continue
		}
		f := prior.Result.Findings[i]
		add(ComparisonItem{State: "not_reobserved", Prior: &f, Changed: []string{}})
	}
	if out.Truncated {
		out.Limitations = append(out.Limitations, "仅显示前 200 条差异，请打开原始运行查看剩余发现。")
	}
	return out
}
func (s *Store) CompareRuns(ctx context.Context, currentID, priorID, actor int64) (RunComparison, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RunComparison{}, err
	}
	defer tx.Rollback()
	// Authorize both fixed snapshots before inspecting their report bodies.
	for _, id := range []int64{currentID, priorID} {
		snap, err := snapshotForRun(ctx, tx, id)
		if err != nil {
			return RunComparison{}, err
		}
		if _, err = requireSnapshotRole(ctx, tx, snap, actor, "viewer"); err != nil {
			return RunComparison{}, err
		}
	}
	current, large, err := associationRun(ctx, tx, currentID)
	if err != nil {
		return RunComparison{}, err
	}
	prior, priorLarge, err := associationRun(ctx, tx, priorID)
	if err != nil {
		return RunComparison{}, err
	}
	if currentID == priorID || current.ProjectID != prior.ProjectID || current.MRIID != prior.MRIID || current.SourceProjectID != prior.SourceProjectID {
		return RunComparison{}, ErrComparisonScope
	}
	if large || priorLarge {
		return RunComparison{Current: comparisonIdentity(current), Prior: comparisonIdentity(prior), Items: []ComparisonItem{}, Truncated: true, Limitations: []string{"至少一份结果超过 1 MiB，请打开原报告人工对照；未加载不代表没有差异。"}}, nil
	}
	out := compareRunFindings(current, prior)
	if err = tx.Commit(); err != nil {
		return RunComparison{}, err
	}
	return out, nil
}
func (h *HTTP) compareRuns(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	prior, err := strconv.ParseInt(c.Query("prior_id"), 10, 64)
	if err != nil || prior <= 0 {
		c.JSON(400, gin.H{"error": "invalid prior_id"})
		return
	}
	out, err := h.Store.CompareRuns(c.Request.Context(), id, prior, currentUser(c).ID)
	if errors.Is(err, ErrComparisonScope) {
		c.JSON(400, gin.H{"error": "runs must share project, source and MR"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, out)
}
