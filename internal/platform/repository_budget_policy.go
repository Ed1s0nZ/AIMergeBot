package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"
)

var ErrRepositoryBudget = errors.New("invalid fixed repository budget")

const repositoryBudgetJSONLimit = 4096

// Limits are an immutable contract, not evidence that a ledger was attached.
type RepositoryBudgetLimits struct {
	Version                 int64 `json:"version"`
	Requests                int64 `json:"requests"`
	APIResponseBytes        int64 `json:"api_response_bytes"`
	GitResponseBytes        int64 `json:"git_response_bytes"`
	UploadBytes             int64 `json:"upload_bytes"`
	DownloadBytes           int64 `json:"download_bytes"`
	GrantBytes              int64 `json:"grant_bytes"`
	PerAPIResponseBytes     int64 `json:"per_api_response_bytes"`
	PerGitResponseBytes     int64 `json:"per_git_response_bytes"`
	ParallelRequests        int64 `json:"parallel_requests"`
	AdmissionTimeoutSeconds int64 `json:"admission_timeout_seconds"`
	TextMatchCells          int64 `json:"text_match_cells"`
	PerTextMatchCells       int64 `json:"per_text_match_cells"`
	MaxReservations         int64 `json:"max_reservations"`
}

func (l *RepositoryBudgetLimits) clone() *RepositoryBudgetLimits {
	if l == nil {
		return nil
	}
	out := *l
	return &out
}

func (l *RepositoryBudgetLimits) UnmarshalJSON(raw []byte) error {
	if len(raw) > repositoryBudgetJSONLimit || !utf8.Valid(raw) || !json.Valid(raw) || uniqueScopeJSON(raw) != nil {
		return ErrRepositoryBudget
	}
	if _, err := scopeObject(raw, "version", "requests", "api_response_bytes", "git_response_bytes", "upload_bytes", "download_bytes", "grant_bytes", "per_api_response_bytes", "per_git_response_bytes", "parallel_requests", "admission_timeout_seconds", "text_match_cells", "per_text_match_cells", "max_reservations"); err != nil {
		return ErrRepositoryBudget
	}
	type plain RepositoryBudgetLimits
	var out plain
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || out.Version != 1 || out.MaxReservations != 8192 {
		return ErrRepositoryBudget
	}
	for _, value := range []int64{out.Requests, out.APIResponseBytes, out.GitResponseBytes, out.UploadBytes, out.DownloadBytes, out.GrantBytes, out.PerAPIResponseBytes, out.PerGitResponseBytes, out.ParallelRequests, out.AdmissionTimeoutSeconds, out.TextMatchCells, out.PerTextMatchCells} {
		if value < 0 {
			return ErrRepositoryBudget
		}
	}
	*l = RepositoryBudgetLimits(out)
	return nil
}

func repositoryBudgetLimitsFor(p *AuditPolicy, purpose string) (*RepositoryBudgetLimits, error) {
	const mib = int64(1 << 20)
	l := &RepositoryBudgetLimits{Version: 1, Requests: 512, APIResponseBytes: 32 * mib, GrantBytes: mib, PerAPIResponseBytes: 8 * mib, ParallelRequests: 1, MaxReservations: 8192}
	switch purpose {
	case "audit", "scope_preview":
		if p == nil || p.TimeoutSeconds <= 0 || p.Git.MaxPackMiB < 16 || p.Git.MaxPackMiB > 4096 {
			return nil, ErrRepositoryBudget
		}
		l.GitResponseBytes = int64(p.Git.MaxPackMiB) * mib
		l.PerGitResponseBytes, l.UploadBytes = l.GitResponseBytes, 8*mib
		l.AdmissionTimeoutSeconds = min(int64(p.TimeoutSeconds), 60)
		if purpose == "audit" {
			l.ParallelRequests = 2
			l.TextMatchCells = 32000000
			l.PerTextMatchCells = 4000000
		}
	case "codeowners":
		l.Requests, l.APIResponseBytes, l.AdmissionTimeoutSeconds = 32, 4*mib, 30
	case "publication":
		l.Requests, l.APIResponseBytes, l.AdmissionTimeoutSeconds = 16, 8*mib, 30
	case "poll":
		l.Requests, l.APIResponseBytes, l.AdmissionTimeoutSeconds = 16, 8*mib, 15
	case "diagnostic":
		l.Requests, l.APIResponseBytes, l.AdmissionTimeoutSeconds = 8, mib, 15
	default:
		return nil, ErrRepositoryBudget
	}
	l.PerAPIResponseBytes = min(l.PerAPIResponseBytes, l.APIResponseBytes)
	l.GrantBytes = min(l.GrantBytes, l.APIResponseBytes)
	l.DownloadBytes = l.APIResponseBytes + l.GitResponseBytes
	return l, nil
}

func (l *RepositoryBudgetLimits) validateFor(p *AuditPolicy, purpose string) error {
	want, err := repositoryBudgetLimitsFor(p, purpose)
	if err != nil || l == nil || *l != *want {
		return ErrRepositoryBudget
	}
	return nil
}
