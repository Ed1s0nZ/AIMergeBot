package platform

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestVerificationContextCoverageMustBeFreshAndCited(t *testing.T) {
	snap := Snapshot{BaseSHA: "base", HeadSHA: "head", AuditPolicy: &AuditPolicy{ContextRepositories: []ContextRepository{{ProjectID: 2, SHA: "ctx"}}}}
	f := Finding{Side: "head", File: "entry.any", Line: 1, Evidence: "sink(input)"}
	raw, _ := json.Marshal(toolOutput{BaseSHA: "base", HeadSHA: "head", Text: "1: sink(input)"})
	primary := ToolTrace{Stage: "verification", Name: "read_file", ObservationID: "v1", Arguments: `{"path":"entry.any"}`, Output: string(raw)}
	input := verificationInput{Checks: supportedChecks("v1"), ClaimCoverage: "full", Status: "supported", Reason: strings.Repeat("界", 1000), Limitations: []string{"Original limitation"}, ObservationIDs: []string{"v1"}}
	ctxRaw, _ := json.Marshal(toolOutput{RepositoryID: 2, BaseSHA: "ctx", HeadSHA: "ctx", Text: "1: guard"})
	related := ToolTrace{Stage: "verification", Name: "read_repository_file", ObservationID: "v2", Output: string(ctxRaw)}
	v, err := validateVerification(input, snap, f, []ToolTrace{primary, related})
	if err != nil || v.Status != "inconclusive" || len(v.Limitations) != 2 || v.Limitations[0] != "Original limitation" || utf8.RuneCountInString(v.Reason) > 1000 {
		t.Fatal("uncited context accepted or limits lost", v, err)
	}
	input.ObservationIDs = append(input.ObservationIDs, "v2")
	v, err = validateVerification(input, snap, f, []ToolTrace{primary, related})
	if err != nil || v.Status != "supported" {
		t.Fatal("fresh context did not satisfy gate", v, err)
	}
	for _, mode := range []string{"old_sha", "directory", "failed"} {
		bad := related
		if mode == "old_sha" {
			bad.Output = `{"repository_id":2,"base_sha":"old","head_sha":"old","text":"source"}`
		}
		if mode == "directory" {
			bad.Name = "list_repository_directory"
		}
		if mode == "failed" {
			bad.Error = "revoked"
		}
		if _, err := validateVerification(input, snap, f, []ToolTrace{primary, bad}); err == nil {
			t.Fatal("invalid context evidence accepted", mode)
		}
	}
	snap.AuditPolicy.ContextRepositories = append(snap.AuditPolicy.ContextRepositories, ContextRepository{ProjectID: 3, SHA: "other"})
	v, err = validateVerification(input, snap, f, []ToolTrace{primary, related})
	if err != nil || v.Status != "inconclusive" || !strings.Contains(v.Reason, "3") {
		t.Fatal("missing second repository ignored", v, err)
	}
}
