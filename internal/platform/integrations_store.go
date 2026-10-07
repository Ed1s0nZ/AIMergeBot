package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"
)

var ErrIntegrationInput = errors.New("invalid integration configuration")

type Integration struct {
	ID              int64    `json:"id"`
	Revision        int64    `json:"revision"`
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	Enabled         bool     `json:"enabled"`
	ProjectIDs      []int    `json:"project_ids"`
	Events          []string `json:"events"`
	Frequency       string   `json:"frequency"`
	MinimumSeverity string   `json:"minimum_severity"`
	HasEndpoint     bool     `json:"has_endpoint"`
	HasSecret       bool     `json:"has_secret"`
	HasJiraMapping  bool     `json:"has_jira_mapping"`
	HasTeamMapping  bool     `json:"has_team_mapping"`
	UpdatedAt       string   `json:"updated_at"`
}
type IntegrationCredentials struct {
	SlackAppID       string   `json:"slack_app_id,omitempty"`
	SlackWorkspaceID string   `json:"slack_workspace_id,omitempty"`
	JiraProjectID    string   `json:"jira_project_id,omitempty"`
	JiraIssueTypeID  string   `json:"jira_issue_type_id,omitempty"`
	LinearTeamID     string   `json:"linear_team_id,omitempty"`
	AllowedNetworks  []string `json:"allowed_networks,omitempty"`
	Endpoint         string   `json:"endpoint"`
	Secret           string   `json:"secret"`
	Token            string   `json:"token"`
	SMTPHost         string   `json:"smtp_host"`
	SMTPPort         int      `json:"smtp_port"`
	SMTPMode         string   `json:"smtp_mode"`
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	From             string   `json:"from"`
	Recipients       []string `json:"recipients"`
}
type IntegrationInput struct {
	Integration
	ExpectedRevision *int64                  `json:"expected_revision"`
	Credentials      *IntegrationCredentials `json:"credentials,omitempty"`
	ClearCredentials bool                    `json:"clear_credentials"`
}

func migrateIntegrations(tx *sql.Tx) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS platform_integrations(id INTEGER PRIMARY KEY,revision INTEGER NOT NULL,name TEXT NOT NULL,kind TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 0,project_ids TEXT NOT NULL,events TEXT NOT NULL,frequency TEXT NOT NULL,minimum_severity TEXT NOT NULL,credentials TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS platform_notification_deliveries(id INTEGER PRIMARY KEY,integration_id INTEGER NOT NULL REFERENCES platform_integrations(id),integration_revision INTEGER NOT NULL,project_id INTEGER NOT NULL,run_id INTEGER NOT NULL DEFAULT 0,event_key TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'pending',attempt INTEGER NOT NULL DEFAULT 0,next_attempt TEXT NOT NULL,lease_until TEXT NOT NULL DEFAULT '',lease_token TEXT NOT NULL DEFAULT '',payload TEXT NOT NULL,error_code TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(integration_id,event_key))`,
		`CREATE INDEX IF NOT EXISTS platform_delivery_ready ON platform_notification_deliveries(status,next_attempt,id)`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
func requireIntegrationAdmin(ctx context.Context, q queryRower, actor int64) error {
	var role string
	var disabled bool
	if err := q.QueryRowContext(ctx, `SELECT role,disabled FROM platform_users WHERE id=?`, actor).Scan(&role, &disabled); err != nil {
		return err
	}
	if disabled {
		return ErrCredentials
	}
	if role != "admin" {
		return ErrProjectPermission
	}
	return nil
}
func validateIntegration(v IntegrationInput) error {
	if v.ExpectedRevision == nil || *v.ExpectedRevision < 0 || strings.TrimSpace(v.Name) == "" || len(v.Name) > 100 || len(v.ProjectIDs) == 0 || len(v.ProjectIDs) > 100 || len(v.Events) > 10 {
		return ErrIntegrationInput
	}
	switch v.Kind {
	case "email", "feishu", "dingtalk", "wecom", "slack", "teams", "webhook", "gitlab", "github", "jira", "linear":
	default:
		return ErrIntegrationInput
	}
	switch v.Frequency {
	case "instant", "daily", "weekly":
	default:
		return ErrIntegrationInput
	}
	switch v.MinimumSeverity {
	case "", "info", "low", "medium", "high", "critical":
	default:
		return ErrIntegrationInput
	}
	seen := map[int]bool{}
	for _, id := range v.ProjectIDs {
		if id <= 0 || seen[id] {
			return ErrIntegrationInput
		}
		seen[id] = true
	}
	events := map[string]bool{}
	for _, e := range v.Events {
		switch e {
		case "run.completed", "run.failed", "finding.reviewed", "risk.expired":
		default:
			return ErrIntegrationInput
		}
		if events[e] {
			return ErrIntegrationInput
		}
		events[e] = true
	}
	if v.Credentials != nil && v.ClearCredentials {
		return ErrIntegrationInput
	}
	return nil
}
func validateIntegrationCredentials(kind string, c IntegrationCredentials, enabled bool) error {
	if c.SlackAppID != "" && !slackAppID.MatchString(c.SlackAppID) || c.SlackWorkspaceID != "" && !slackWorkspaceID.MatchString(c.SlackWorkspaceID) {
		return ErrIntegrationInput
	}
	if c.JiraProjectID != "" && !jiraNumericID.MatchString(c.JiraProjectID) || c.JiraIssueTypeID != "" && !jiraNumericID.MatchString(c.JiraIssueTypeID) {
		return ErrIntegrationInput
	}
	if c.LinearTeamID != "" && !ticketUUID.MatchString(c.LinearTeamID) {
		return ErrIntegrationInput
	}
	if len(c.AllowedNetworks) > 10 {
		return ErrIntegrationInput
	}
	for _, value := range c.AllowedNetworks {
		ip, block, err := net.ParseCIDR(value)
		if err != nil || !ip.IsPrivate() {
			return ErrIntegrationInput
		}
		ones, _ := block.Mask.Size()
		if ones < 8 {
			return ErrIntegrationInput
		}
	}
	if len(c.Endpoint) > 4096 || len(c.Secret) > 4096 || len(c.Token) > 4096 || len(c.Password) > 4096 || len(c.Username) > 256 {
		return ErrIntegrationInput
	}
	if kind != "email" {
		if c.Endpoint == "" {
			if enabled {
				return ErrIntegrationInput
			}
			return nil
		}
		u, err := url.Parse(c.Endpoint)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return ErrIntegrationInput
		}
		return nil
	}
	if c.SMTPHost == "" {
		if enabled {
			return ErrIntegrationInput
		}
		return nil
	}
	if strings.ContainsAny(c.SMTPHost, "/\r\n ") || c.SMTPPort < 1 || c.SMTPPort > 65535 || (c.SMTPMode != "tls" && c.SMTPMode != "starttls") || len(c.Recipients) == 0 || len(c.Recipients) > 50 {
		return ErrIntegrationInput
	}
	for _, address := range append(append([]string{}, c.Recipients...), c.From) {
		if strings.ContainsAny(address, "\r\n") {
			return ErrIntegrationInput
		}
		a, err := mail.ParseAddress(address)
		if err != nil || a.Address != address {
			return ErrIntegrationInput
		}
	}
	return nil
}
func scanIntegration(row interface{ Scan(...any) error }) (Integration, IntegrationCredentials, error) {
	var v Integration
	var projects, events, secret string
	err := row.Scan(&v.ID, &v.Revision, &v.Name, &v.Kind, &v.Enabled, &projects, &events, &v.Frequency, &v.MinimumSeverity, &secret, &v.UpdatedAt)
	if err != nil {
		return v, IntegrationCredentials{}, err
	}
	var c IntegrationCredentials
	for _, item := range []struct {
		s string
		v any
	}{{projects, &v.ProjectIDs}, {events, &v.Events}, {secret, &c}} {
		if err = json.Unmarshal([]byte(item.s), item.v); err != nil {
			return v, c, err
		}
	}
	v.HasEndpoint = c.Endpoint != "" || c.SMTPHost != ""
	v.HasSecret = c.Secret != "" || c.Token != "" || c.Password != ""
	v.HasTeamMapping = v.Kind == "linear" && c.LinearTeamID != ""
	v.HasJiraMapping = v.Kind == "jira" && c.JiraProjectID != "" && c.JiraIssueTypeID != ""
	return v, c, nil
}

const integrationColumns = `id,revision,name,kind,enabled,project_ids,events,frequency,minimum_severity,credentials,updated_at`

func (s *Store) Integrations(ctx context.Context, actor int64) ([]Integration, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = requireIntegrationAdmin(ctx, tx, actor); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations ORDER BY id DESC LIMIT 201`)
	if err != nil {
		return nil, err
	}
	items := []Integration{}
	for rows.Next() {
		v, _, err := scanIntegration(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(items) > 200 {
		return nil, ErrIntegrationInput
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}
func (s *Store) SaveIntegration(ctx context.Context, id, actor int64, input IntegrationInput) (Integration, error) {
	if err := validateIntegration(input); err != nil {
		return Integration{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Integration{}, err
	}
	defer tx.Rollback()
	if err = requireIntegrationAdmin(ctx, tx, actor); err != nil {
		return Integration{}, err
	}
	var old Integration
	var credentials IntegrationCredentials
	if id > 0 {
		old, credentials, err = scanIntegration(tx.QueryRowContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE id=?`, id))
		if err != nil {
			return Integration{}, err
		}
		if old.Revision != *input.ExpectedRevision || old.Kind != input.Kind {
			return Integration{}, ErrConflict
		}
	} else {
		if *input.ExpectedRevision != 0 {
			return Integration{}, ErrConflict
		}
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_integrations`).Scan(&count); err != nil {
			return Integration{}, err
		}
		if count >= 200 {
			return Integration{}, ErrIntegrationInput
		}
	}
	if input.ClearCredentials {
		credentials = IntegrationCredentials{}
	} else if input.Credentials != nil {
		credentials = *input.Credentials
	}
	if err = validateIntegrationCredentials(input.Kind, credentials, input.Enabled); err != nil {
		return Integration{}, err
	}
	for _, project := range input.ProjectIDs {
		var exists int
		if err = tx.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&exists); err != nil {
			return Integration{}, err
		}
	}
	sort.Ints(input.ProjectIDs)
	projects, _ := json.Marshal(input.ProjectIDs)
	events, _ := json.Marshal(input.Events)
	secret, _ := json.Marshal(credentials)
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	revision := old.Revision + 1
	if id == 0 {
		res, e := tx.ExecContext(ctx, `INSERT INTO platform_integrations(revision,name,kind,enabled,project_ids,events,frequency,minimum_severity,credentials,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, revision, strings.TrimSpace(input.Name), input.Kind, input.Enabled, string(projects), string(events), input.Frequency, input.MinimumSeverity, string(secret), updated)
		if e != nil {
			return Integration{}, e
		}
		id, err = res.LastInsertId()
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE platform_integrations SET revision=?,name=?,enabled=?,project_ids=?,events=?,frequency=?,minimum_severity=?,credentials=?,updated_at=? WHERE id=?`, revision, strings.TrimSpace(input.Name), input.Enabled, string(projects), string(events), input.Frequency, input.MinimumSeverity, string(secret), updated, id)
	}
	if err != nil {
		return Integration{}, err
	}
	// Saved destination changes invalidate pending payloads; never resend them to a new recipient.
	if _, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status='unknown',error_code='configuration_changed',lease_token='',lease_until='',updated_at=? WHERE integration_id=? AND status='sending'`, updated, id); err != nil {
		return Integration{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE platform_notification_deliveries SET status='cancelled',error_code='configuration_changed',updated_at=? WHERE integration_id=? AND status IN ('pending','retry')`, updated, id); err != nil {
		return Integration{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'integration.saved',?,?)`, actor, id, updated); err != nil {
		return Integration{}, err
	}
	if err = tx.Commit(); err != nil {
		return Integration{}, err
	}
	v := input.Integration
	v.ID = id
	v.Revision = revision
	v.Name = strings.TrimSpace(v.Name)
	v.UpdatedAt = updated
	v.HasEndpoint = credentials.Endpoint != "" || credentials.SMTPHost != ""
	v.HasSecret = credentials.Secret != "" || credentials.Token != "" || credentials.Password != ""
	v.HasTeamMapping = v.Kind == "linear" && credentials.LinearTeamID != ""
	v.HasJiraMapping = v.Kind == "jira" && credentials.JiraProjectID != "" && credentials.JiraIssueTypeID != ""
	return v, nil
}
