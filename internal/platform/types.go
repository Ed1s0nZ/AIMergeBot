package platform

import "time"

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

type Project struct {
	ContextRepositories []ContextRepository `json:"context_repositories,omitempty"`
	AccessRole          string              `json:"access_role,omitempty"`
	ID                  int                 `json:"id"`
	Name                string              `json:"name"`
	Enabled             bool                `json:"enabled"`
}

type Finding struct {
	PRContext       *PRInvestigationContext `json:"pr_context,omitempty"`
	Fingerprint     string                  `json:"fingerprint,omitempty"`
	Verification    *FindingVerification    `json:"verification,omitempty"`
	AnchorType      string                  `json:"anchor_type,omitempty"`
	Metadata        *GitChangeMetadata      `json:"metadata,omitempty"`
	InvestigationID string                  `json:"investigation_id,omitempty"`
	ObservationIDs  []string                `json:"observation_ids,omitempty"`
	SequenceDiagram *SequenceDiagram        `json:"sequence_diagram,omitempty"`
	Side            string                  `json:"side"`
	Type            string                  `json:"type"`
	ID              string                  `json:"id"`
	File            string                  `json:"file"`
	Line            int                     `json:"line"`
	Severity        string                  `json:"severity"`
	Title           string                  `json:"title"`
	Description     string                  `json:"description"`
	Evidence        string                  `json:"evidence"`
	Trigger         string                  `json:"trigger"`
	Suggestion      string                  `json:"suggestion"`
	Confidence      string                  `json:"confidence"`
}

type Investigation struct {
	Plan                  []InvestigationTask     `json:"plan,omitempty"`
	PRContext             *PRInvestigationContext `json:"pr_context,omitempty"`
	ObservationIDs        []string                `json:"observation_ids,omitempty"`
	CounterObservationIDs []string                `json:"counter_observation_ids,omitempty"`
	ID                    string                  `json:"id"`
	Claim                 string                  `json:"claim"`
	Status                string                  `json:"status"`
	Evidence              []string                `json:"evidence"`
	Counterevidence       []string                `json:"counterevidence"`
	NextSteps             []string                `json:"next_steps"`
}
type AuditResult struct {
	AuditGroups     []AuditGroupProgress `json:"audit_groups,omitempty"`
	MetadataChanges []GitChangeMetadata  `json:"metadata_changes,omitempty"`
	ExcludedFiles   []string             `json:"excluded_files,omitempty"`
	Investigations  []Investigation      `json:"investigations,omitempty"`
	Findings        []Finding            `json:"findings"`
	Summary         string               `json:"summary"`
	CoverageNotes   []string             `json:"coverage_notes"`
}

type Snapshot struct {
	AuditPolicy     *AuditPolicy `json:"audit_policy,omitempty"`
	DiffVersionID   int          `json:"diff_version_id"`
	ProjectID       int          `json:"project_id"`
	SourceProjectID int          `json:"source_project_id"`
	MRIID           int          `json:"mr_iid"`
	BaseSHA         string       `json:"base_sha"`
	HeadSHA         string       `json:"head_sha"`
	Title           string       `json:"title"`
	URL             string       `json:"url"`
}

type Run struct {
	RetryInfo        *RetryInfo `json:"retry_info,omitempty"`
	WorkerOwner      string     `json:"-"`
	WorkerLeaseUntil string     `json:"worker_lease_until,omitempty"`
	RetryChildID     int64      `json:"retry_child_id,omitempty"`
	RetryParentID    int64      `json:"retry_parent_id,omitempty"`
	RetryAttempt     int        `json:"retry_attempt"`
	RetryAt          string     `json:"retry_at,omitempty"`
	ID               int64      `json:"id"`
	Snapshot
	Status        string      `json:"status"`
	Error         string      `json:"error"`
	Result        AuditResult `json:"result"`
	Trace         []ToolTrace `json:"trace"`
	CreatedAt     time.Time   `json:"created_at"`
	StartedAt     *time.Time  `json:"started_at,omitempty"`
	FinishedAt    *time.Time  `json:"finished_at,omitempty"`
	RequestedBy   int64       `json:"requested_by"`
	PolicyVersion string      `json:"policy_version"`
}

type ToolTrace struct {
	Stage            string `json:"stage,omitempty"`
	Output           string `json:"output,omitempty"`
	ObservationID    string `json:"observation_id,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	UsageReported    bool   `json:"usage_reported,omitempty"`
	Partial          bool   `json:"partial,omitempty"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	DurationMS       int64  `json:"duration_ms"`
	Error            string `json:"error,omitempty"`
}

type Review struct {
	Revision         int64  `json:"revision"`
	ExpectedRevision *int64 `json:"expected_revision,omitempty"`
	RunID            int64  `json:"run_id"`
	FindingID        string `json:"finding_id"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
	Actor            int64  `json:"actor"`
	UpdatedAt        string `json:"updated_at"`
}
