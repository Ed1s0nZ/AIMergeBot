package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunUsageHTTPUsesFrozenPrices(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Bootstrap(ctx, "admin", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProject(ctx, Project{ID: 1, Name: "fixture", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(ctx, "admin", "a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{ModelBudget: ModelBudgetSettings{Currency: "USD", InputPricePerMillion: 2, OutputPricePerMillion: 8}}}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, id, "succeeded", "", AuditResult{Findings: []Finding{}, Summary: "fixture", CoverageNotes: []string{}}, []ToolTrace{{Name: "model", UsageReported: true, PromptTokens: 1000000, CompletionTokens: 1000000}}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	(&HTTP{Store: s, Runner: &Runner{Store: s}}).Register(router)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/runs/%d", id), nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var response struct {
		Usage ModelUsage `json:"usage"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("status=%d", w.Code)
	}
	if !response.Usage.Complete || response.Usage.Currency != "USD" || response.Usage.EstimatedCost == nil || *response.Usage.EstimatedCost != 10 {
		t.Fatalf("bad frozen cost: %+v", response.Usage)
	}
}
