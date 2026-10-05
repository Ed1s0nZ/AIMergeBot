package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// AuditPolicy is immutable per run; it deliberately contains no credentials.
type AuditPolicy struct {
	ModelBudget      ModelBudgetSettings `json:"model_budget"`
	RepositoryURL    string              `json:"repository_url"`
	ModelURL         string              `json:"model_url"`
	Model            string              `json:"model"`
	Temperature      float32             `json:"temperature"`
	MaxSteps         int                 `json:"max_steps"`
	TimeoutSeconds   int                 `json:"timeout_seconds"`
	Git              GitAuditSettings    `json:"git"`
	Excluded         []string            `json:"excluded"`
	VerifyFindings   bool                `json:"verify_findings"`
	GenerateDiagrams bool                `json:"generate_diagrams"`
}

func capturePolicy(s Settings) *AuditPolicy {
	model := s.ReAct.Model
	if model == "" {
		model = s.OpenAI.Model
	}
	return &AuditPolicy{ModelBudget: s.ModelBudget, RepositoryURL: strings.TrimRight(s.GitLab.URL, "/"), ModelURL: s.OpenAI.URL, Model: model, Temperature: float32(s.ReAct.Temperature), MaxSteps: s.ReAct.MaxSteps, TimeoutSeconds: s.AuditTimeoutSeconds, Git: s.GitAudit, Excluded: append([]string{}, s.WhitelistExtensions...), GenerateDiagrams: s.GenerateSequenceDiagrams, VerifyFindings: s.VerifyFindings}
}
func policyDigest(p *AuditPolicy) string {
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
