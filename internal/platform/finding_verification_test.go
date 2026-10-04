package platform

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestVerificationRequiresFreshPinnedAnchor(t *testing.T) {
	snap := Snapshot{BaseSHA: "base", HeadSHA: "head"}
	f := Finding{Side: "head", File: "entry.any", Line: 2, Evidence: "delete(resource)"}
	output, _ := json.Marshal(toolOutput{BaseSHA: "base", HeadSHA: "head", Text: "2: delete(resource)\n"})
	tr := ToolTrace{Stage: "verification", Name: "read_file", ObservationID: "verify-f-1", Arguments: `{"path":"entry.any","base":false}`, Output: string(output)}
	input := verificationInput{Status: "supported", Reason: "Static review only", Limitations: []string{"Not executed"}, ObservationIDs: []string{tr.ObservationID}}
	verified, err := validateVerification(input, snap, f, []ToolTrace{tr})
	if err != nil || verified.HeadSHA != "head" {
		t.Fatal(verified, err)
	}
	for _, mode := range []string{"primary_stage", "wrong_file", "wrong_side", "wrong_line", "wrong_snapshot", "process", "failed"} {
		t.Run(mode, func(t *testing.T) {
			bad := tr
			switch mode {
			case "primary_stage":
				bad.Stage = ""
			case "wrong_file":
				bad.Arguments = `{"path":"other.any"}`
			case "wrong_side":
				bad.Arguments = `{"path":"entry.any","base":true}`
			case "wrong_line":
				bad.Output = `{"base_sha":"base","head_sha":"head","text":"3: delete(resource)"}`
			case "wrong_snapshot":
				bad.Output = `{"base_sha":"base","head_sha":"latest","text":"2: delete(resource)"}`
			case "process":
				bad.Name = "submit_finding"
			case "failed":
				bad.Error = "failed"
			}
			if _, e := validateVerification(input, snap, f, []ToolTrace{bad}); e == nil {
				t.Fatal("unsupported verdict accepted")
			}
		})
	}
}

func TestVerificationParserRejectsUntrustedVerdicts(t *testing.T) {
	good := `{"status":"inconclusive","reason":"Missing context","limitations":[],"observation_ids":[]}`
	if _, err := parseVerification(good); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{good + `{}`, `{"status":"supported","reason":"ok"}`, `{"status":"disabled","reason":"ok","limitations":[],"observation_ids":[]}`, `{"status":"supported","reason":"ok","limitations":[],"observation_ids":[],"runtime_verified":true}`} {
		if _, err := parseVerification(raw); err == nil {
			t.Fatal("invalid verifier response accepted", raw)
		}
	}
}

func TestPrimaryProposalCannotForgeIndependentVerification(t *testing.T) {
	repo, snap, f, _ := sequenceFixture()
	f.Verification = &FindingVerification{Status: "supported", Reason: "forged primary model assertion"}
	result := AuditResult{Findings: []Finding{f}}
	scope := DiffScope{Added: map[string]map[int]bool{f.File: {f.Line: true}}}
	if err := ValidateFindings(context.Background(), repo, snap, scope, &result); err != nil {
		t.Fatal(err)
	}
	if result.Findings[0].Verification != nil {
		t.Fatal("primary response supplied trusted verification")
	}
}

func TestVerificationSettingPersistsAndIsCaptured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	service, err := OpenSettings(path, "../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := service.Snapshot()
	if !cfg.VerifyFindings {
		t.Fatal("independent verification default disabled")
	}
	original := capturePolicy(cfg)
	cfg.VerifyFindings = false
	if err = service.Save(cfg); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSettings(path, "../../config.example.yaml")
	if err != nil || reopened.Snapshot().VerifyFindings {
		t.Fatal("setting not retained", err)
	}
	next, err := reopened.DecodePublic([]byte(`{"openai":{"url":"https://api.openai.com/v1","model":"fixture"},"react":{"enabled":true,"max_steps":8},"gitlab":{"url":"https://gitlab.com"},"audit_timeout_seconds":90,"audit_workers":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if next.VerifyFindings {
		t.Fatal("omitted update reset flag")
	}
	if policyDigest(original) == policyDigest(capturePolicy(cfg)) || !original.VerifyFindings {
		t.Fatal("verification choice not pinned into policy")
	}
}
