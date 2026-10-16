package platform

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func repositoryBudgetPolicyFixture() *AuditPolicy {
	return &AuditPolicy{TimeoutSeconds: 120, Git: GitAuditSettings{MaxPackMiB: 256}}
}

func TestRepositoryBudgetPolicyStrictAndFrozen(t *testing.T) {
	p := repositoryBudgetPolicyFixture()
	l, err := repositoryBudgetLimitsFor(p, "audit")
	if err != nil || l.DownloadBytes != 288<<20 || l.AdmissionTimeoutSeconds != 60 || l.ParallelRequests != 2 {
		t.Fatal(l, err)
	}
	p.RepositoryBudget = l
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var out AuditPolicy
	if err = json.Unmarshal(raw, &out); err != nil || out.RepositoryBudget == nil || *out.RepositoryBudget != *l {
		t.Fatal(err)
	}
	before := policyDigest(p)
	copy := l.clone()
	copy.Requests--
	if l.Requests != 512 || copy.validateFor(p, "audit") == nil {
		t.Fatal("limits mutated or mismatch accepted")
	}
	changed := *p
	changed.RepositoryBudget = copy
	if policyDigest(&changed) == before {
		t.Fatal("budget missing from digest")
	}
	legacy := *p
	legacy.RepositoryBudget = nil
	oldJSON, _ := json.Marshal(&legacy)
	if strings.Contains(string(oldJSON), "repository_budget") {
		t.Fatal("changed old policy JSON")
	}
	for name, body := range map[string]string{
		"null":                strings.Replace(string(raw), `"repository_budget":{`, `"repository_budget":null,"unexpected":{`, 1),
		"case alias":          strings.Replace(string(raw), `"repository_budget":`, `"REPOSITORY_BUDGET":`, 1),
		"wrong formula":       strings.Replace(string(raw), `"requests":512`, `"requests":513`, 1),
		"float":               strings.Replace(string(raw), `"requests":512`, `"requests":512.0`, 1),
		"overflow":            strings.Replace(string(raw), `"requests":512`, `"requests":9223372036854775808`, 1),
		"negative":            strings.Replace(string(raw), `"requests":512`, `"requests":-1`, 1),
		"duplicate":           strings.Replace(string(raw), `"requests":512`, `"requests":512,"requests":512`, 1),
		"unknown":             strings.Replace(string(raw), `"requests":512`, `"requests":512,"PRIVATE":true`, 1),
		"missing":             strings.Replace(string(raw), `"requests":512,`, ``, 1),
		"wrong revision":      strings.Replace(string(raw), `"version":1`, `"version":2`, 1),
		"changed frozen pack": strings.Replace(string(raw), `"max_pack_mib":256`, `"max_pack_mib":16`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			var v AuditPolicy
			if err := json.Unmarshal([]byte(body), &v); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("accepted malformed budget or leaked data", err)
			}
		})
	}
	for _, pack := range []int{0, 15, 4097, math.MaxInt} {
		bad := *p
		bad.Git.MaxPackMiB = pack
		if _, err := repositoryBudgetLimitsFor(&bad, "audit"); !errors.Is(err, ErrRepositoryBudget) {
			t.Fatal(pack, err)
		}
	}
	p.Git.MaxPackMiB = 4096
	p.TimeoutSeconds = 10
	large, err := repositoryBudgetLimitsFor(p, "audit")
	if err != nil || large.GitResponseBytes != int64(4096)<<20 || large.AdmissionTimeoutSeconds != 10 {
		t.Fatal("overflow or timeout", err)
	}
	standalone, _ := json.Marshal(l)
	if json.Unmarshal([]byte("{"+strings.Repeat(" ", repositoryBudgetJSONLimit)+string(standalone[1:])), new(RepositoryBudgetLimits)) == nil {
		t.Fatal("oversized JSON accepted")
	}
}

func TestRepositoryBudgetAuxiliaryProfilesDoNotExpandAudit(t *testing.T) {
	p := repositoryBudgetPolicyFixture()
	for _, tc := range []struct {
		purpose                string
		requests, api, seconds int64
	}{{"codeowners", 32, 4 << 20, 30}, {"publication", 16, 8 << 20, 30}, {"poll", 16, 8 << 20, 15}, {"diagnostic", 8, 1 << 20, 15}} {
		l, err := repositoryBudgetLimitsFor(nil, tc.purpose)
		if err != nil || l.Requests != tc.requests || l.APIResponseBytes != tc.api || l.AdmissionTimeoutSeconds != tc.seconds || l.GitResponseBytes != 0 || l.UploadBytes != 0 || l.TextMatchCells != 0 || l.PerTextMatchCells != 0 || l.ParallelRequests != 1 || l.DownloadBytes != l.APIResponseBytes || l.PerAPIResponseBytes > l.APIResponseBytes {
			t.Fatal(tc.purpose, l, err)
		}
		if l.validateFor(nil, tc.purpose) != nil || l.validateFor(p, "audit") == nil {
			t.Fatal("purpose confused")
		}
	}
	scope, err := repositoryBudgetLimitsFor(p, "scope_preview")
	if err != nil || scope.GitResponseBytes != 256<<20 || scope.TextMatchCells != 0 || scope.ParallelRequests != 1 {
		t.Fatal(scope, err)
	}
	if _, err = repositoryBudgetLimitsFor(p, "unknown"); err == nil {
		t.Fatal("unknown purpose accepted")
	}
}

func TestRepositoryBudgetCannotFallThroughLegacyAdmission(t *testing.T) {
	s, admin, _ := accessFixture(t)
	p := repositoryBudgetPolicyFixture()
	p.RepositoryBudget, _ = repositoryBudgetLimitsFor(p, "audit")
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 3, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), AuditPolicy: p}
	if _, _, err := s.EnqueueUser(context.Background(), snap, admin.ID, false); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("budget-only legacy admission", err)
	}
	snap.AuditPolicy = nil
	id, _, err := s.EnqueueUser(context.Background(), snap, admin.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	injectRepositoryScope(t, s, id, p)
	if err = requireLegacyRunRepository(context.Background(), s.DB, id); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatal("durable budget-only guard", err)
	}
}
