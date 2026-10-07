package platform

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkspaceUsageConservativeReceipts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := json.Marshal(AuditPolicy{Model: "snapshot-model", ModelBudget: ModelBudgetSettings{Currency: "USD", InputPricePerMillion: 2}})
	trace, _ := json.Marshal([]ToolTrace{{Name: "model", UsageReported: true, PromptTokens: 100, CompletionTokens: 20}})
	if _, err = s.DB.Exec(`UPDATE platform_runs SET status='incomplete',error='PRIVATE failure',audit_policy_json=?,trace_json=?,started_at='2026-10-01T01:00:00Z',finished_at='2026-10-01T01:00:10Z' WHERE id=?`, string(policy), string(trace), id); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.workspaceUsage(ctx, " WHERE 1=1", nil, 1, 20)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("%+v %d %v", items, total, err)
	}
	item := items[0]
	if item.Model != "snapshot-model" || item.Usage == nil || item.Usage.Complete || item.Usage.PromptTokens != 100 || item.DurationSeconds == nil || *item.DurationSeconds != 10 {
		t.Fatalf("%+v", item)
	}
	body, _ := json.Marshal(items)
	if strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "trace_json") {
		t.Fatal("raw receipt leaked")
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET trace_json=? WHERE id=?`, strings.Repeat("x", 1048577), id); err != nil {
		t.Fatal(err)
	}
	items, _, err = s.workspaceUsage(ctx, " WHERE 1=1", nil, 1, 20)
	if err != nil || items[0].UnknownReason != "trace_exceeds_read_budget" || items[0].Model != "snapshot-model" || items[0].Usage != nil {
		t.Fatalf("%+v %v", items, err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json='invalid',trace_json='[]' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	items, _, err = s.workspaceUsage(ctx, " WHERE 1=1", nil, 1, 20)
	if err != nil || items[0].Model != "unknown" || items[0].UnknownReason != "invalid_historical_receipt" {
		t.Fatalf("%+v %v", items, err)
	}
}

func TestWorkspaceMetricsSnapshotACLAndPolicyGroups(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "reader", "a-long-password", "member")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range []int{1, 2} {
		if _, err = s.DB.Exec(`INSERT INTO platform_projects(id,name,enabled) VALUES(?,'fixture',1)`, project); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(1,?,'viewer')`, u.ID); err != nil {
		t.Fatal(err)
	}
	for i, policy := range []string{`{"model":"same","max_steps":10}`, `{"model":"same","max_steps":20}`, `invalid`} {
		id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 2, MRIID: i + 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE platform_runs SET audit_policy_json=?,result_json='{"findings":[{"id":"f1","severity":"high","title":"PRIVATE"}]}' WHERE id=?`, policy, id); err != nil {
			t.Fatal(err)
		}
	}
	where, args := workspaceACL(u)
	items, _, err := s.workspaceQuality(ctx, where, args)
	if err != nil || len(items) != 0 {
		t.Fatalf("unauthorized quality %+v %v", items, err)
	}
	_, total, err := s.workspaceUsage(ctx, where, args, 1, 20)
	if err != nil || total != 0 {
		t.Fatalf("unauthorized usage %d %v", total, err)
	}
	if _, err = s.DB.Exec(`INSERT INTO platform_project_members(project_id,user_id,role) VALUES(2,?,'viewer')`, u.ID); err != nil {
		t.Fatal(err)
	}
	items, truncated, err := s.workspaceQuality(ctx, where, args)
	if err != nil || truncated || len(items) != 3 {
		t.Fatalf("groups %+v %v", items, err)
	}
	snapshots := map[string]bool{}
	for _, item := range items {
		if item.Findings != 1 || item.Pending != 1 || len(item.Snapshot) != 64 {
			t.Fatalf("%+v", item)
		}
		snapshots[item.Snapshot] = true
	}
	if len(snapshots) != 3 {
		t.Fatal("different policies combined")
	}
	body, _ := json.Marshal(items)
	if strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "max_steps") {
		t.Fatal("private policy or finding leaked")
	}
	_, total, err = s.workspaceUsage(ctx, where, args, 2, 2)
	if err != nil || total != 3 {
		t.Fatalf("paging %d %v", total, err)
	}
}

func TestWorkspaceMetricsTimeWindowValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		query string
		valid bool
	}{
		{"from=bad", false},
		{"from=2026-10-02T00:00:00Z&to=2026-10-01T00:00:00Z", false},
		{"from=2025-01-01T00:00:00Z&to=2026-10-01T00:00:00Z", false},
		{"project_id=-1", false},
		{"from=2026-10-01T08:00:00%2B08:00&to=2026-10-02T00:00:00Z", true},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/workspace/usage?"+tc.query, nil)
		c.Set("user", User{ID: 1, Role: "admin"})
		_, _, window, ok := metricsScope(c)
		if ok != tc.valid {
			t.Fatalf("%s ok=%v body=%s", tc.query, ok, w.Body.String())
		}
		if ok && window.From != "2026-10-01T00:00:00Z" {
			t.Fatalf("not normalized: %+v", window)
		}
	}
}
