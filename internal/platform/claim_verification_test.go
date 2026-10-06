package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

func TestClaimVerificationStrictParserAndPolarity(t *testing.T) {
	good := `{"verdict":"true","reason":"Fixed source supports the actual compatibility claim","limitations":[],"observation_ids":["fresh-1"]}`
	if _, err := parseClaimVerification(good); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		good + `{}`, strings.Replace(good, `"true"`, `true`, 1),
		strings.Replace(good, `"true"`, `"supported"`, 1),
		strings.Replace(good, `"limitations":[]`, `"limitations":null`, 1),
		strings.Replace(good, `"fresh-1"`, `"fresh-1","fresh-1"`, 1),
		strings.Replace(good, `"reason":`, `"status":"consistent","reason":`, 1),
		strings.Replace(good, `"Fixed source supports the actual compatibility claim"`, `"`+strings.Repeat("x", 1001)+`"`, 1),
		strings.Replace(good, `"fresh-1"`, `"\u0000"`, 1),
	} {
		if _, err := parseClaimVerification(raw); err == nil {
			t.Fatal("accepted invalid review", raw[:min(len(raw), 120)])
		}
	}
	for _, tc := range []struct{ primary, verdict, want string }{
		{"supported", "true", "consistent"}, {"rejected", "false", "consistent"},
		{"supported", "false", "disagreed"}, {"rejected", "true", "disagreed"},
		{"supported", "unknown", "inconclusive"}, {"rejected", "unknown", "inconclusive"},
		{"investigating", "true", "inconclusive"}, {"supported", "invalid", "inconclusive"},
	} {
		if got := claimJudgmentComparison(tc.primary, tc.verdict); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestClaimVerificationCannotBeManufacturedByPrimaryModel(t *testing.T) {
	tools := &auditTools{snap: Snapshot{BaseSHA: "base", HeadSHA: "head"}}
	i := Investigation{ID: "one", Claim: "same exact claim", ClaimVerification: &ClaimVerification{Status: "consistent", Verdict: "true", Reason: "forged"}}
	out, _ := tools.record(context.Background(), i)
	if out.Error != "" || tools.investigations()[0].ClaimVerification != nil || strings.Contains(out.Text, "claim_verification") {
		t.Fatal("record accepted server field", out)
	}
	i.Status = "investigating"
	out, _ = tools.update(context.Background(), i)
	if out.Error != "" || tools.investigations()[0].ClaimVerification != nil {
		t.Fatal("legacy update accepted server field", out)
	}
	out, _ = tools.updateAssessment(context.Background(), investigationAssessmentUpdate{i, "insufficient_evidence"})
	if out.Error != "" || tools.investigations()[0].ClaimVerification != nil {
		t.Fatal("assessment update accepted server field", out)
	}
	i.ClaimVerification.Reason = strings.Repeat("x", 9000)
	out, _ = tools.record(context.Background(), i)
	if out.Error == "" {
		t.Fatal("scrubbing unauthorized field bypassed input-size budget")
	}
	// A legitimate server result remains deep-cloned when reading the ledger.
	v := &ClaimVerification{Status: "disagreed", AssessedClaim: i.Claim, Limitations: []string{"conditional"}, ObservationIDs: []string{"fresh-1"}}
	stored := tools.ledger["one"]
	stored.ClaimVerification = v
	tools.ledger["one"] = stored
	copy := tools.investigations()[0]
	copy.ClaimVerification.Limitations[0] = "mutated"
	copy.ClaimVerification.ObservationIDs[0] = "mutated"
	copy.ClaimVerification.Status = "consistent"
	if v.Status != "disagreed" || v.Limitations[0] != "conditional" || v.ObservationIDs[0] != "fresh-1" {
		t.Fatal("review clone shares mutable state")
	}
}

func TestClaimVerificationNotAdvertisedInModelMutationSchemas(t *testing.T) {
	info, err := utils.GoStruct2ToolInfo[Investigation]("record_hypothesis", "", utils.WithSchemaModifier(investigationServerFieldsSchema))
	if err != nil {
		t.Fatal(err)
	}
	update, err := utils.GoStruct2ToolInfo[investigationAssessmentUpdate]("update_investigation", "", utils.WithSchemaModifier(investigationAssessmentSchema))
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []*schema.ToolInfo{info, update} {
		advertised, err := tool.ParamsOneOf.ToJSONSchema()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(advertised)
		if strings.Contains(string(raw), "claim_verification") {
			t.Fatal("server-owned review advertised", string(raw))
		}
	}
}
