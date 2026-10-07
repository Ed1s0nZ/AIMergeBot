package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// RepositoryBinding maps a platform-independent project to one remote identity.
// It contains no credentials and is not proof of upstream membership.
type RepositoryBinding struct {
	Revision      int64  `json:"revision"`
	Provider      string `json:"provider"`
	APIOrigin     string `json:"api_origin"`
	RemoteID      int64  `json:"remote_id"`
	FullName      string `json:"full_name"`
	IntegrationID int64  `json:"integration_id"`
}

func migrateRepositoryBindings(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_project_repositories(project_id INTEGER PRIMARY KEY REFERENCES platform_projects(id),revision INTEGER NOT NULL,provider TEXT NOT NULL,api_origin TEXT NOT NULL,remote_id INTEGER NOT NULL,binding_json TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(provider,api_origin,remote_id));CREATE TABLE IF NOT EXISTS platform_repository_binding_history(project_id INTEGER NOT NULL REFERENCES platform_projects(id),revision INTEGER NOT NULL,binding_json TEXT NOT NULL,actor INTEGER NOT NULL REFERENCES platform_users(id),created_at TEXT NOT NULL,PRIMARY KEY(project_id,revision))`)
	return err
}

var repositoryNameSegment = regexp.MustCompile(`\A[A-Za-z0-9_.-]+\z`)

var githubRepositoryOwner = regexp.MustCompile(`\A[A-Za-z0-9][A-Za-z0-9-]*\z`)

func canonicalRepositoryAPIOrigin(provider, raw string) (string, error) {
	if len(raw) > 2048 || strings.ContainsAny(raw, "#%\\\r\n\t ") {
		return "", ErrConflict
	}
	for _, ch := range raw {
		if unicode.IsControl(ch) || unicode.IsSpace(ch) {
			return "", ErrConflict
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return "", ErrConflict
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", ErrConflict
		}
		u.Host = strings.TrimSuffix(u.Host, ":"+port)
		if number != 443 {
			u.Host += ":" + strconv.Itoa(number)
		}
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	if provider != "github" && provider != "gitlab" || u.Path != "" && !(provider == "github" && u.Path == "/api/v3") {
		return "", ErrConflict
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

func validateRepositoryBinding(p RepositoryBinding) (RepositoryBinding, error) {
	if p.RemoteID <= 0 || p.IntegrationID <= 0 || len(p.FullName) > 255 {
		return RepositoryBinding{}, ErrConflict
	}
	parts := strings.Split(p.FullName, "/")
	if len(parts) < 2 || p.Provider == "github" && len(parts) != 2 {
		return RepositoryBinding{}, ErrConflict
	}
	if p.Provider == "github" && !githubRepositoryOwner.MatchString(parts[0]) {
		return RepositoryBinding{}, ErrConflict
	}
	for _, part := range parts {
		if !repositoryNameSegment.MatchString(part) || part == "." || part == ".." {
			return RepositoryBinding{}, ErrConflict
		}
	}
	origin, err := canonicalRepositoryAPIOrigin(p.Provider, p.APIOrigin)
	if err != nil {
		return RepositoryBinding{}, err
	}
	p.APIOrigin = origin
	return p, nil
}

func readRepositoryBinding(ctx context.Context, q queryRower, project int) (RepositoryBinding, error) {
	var p RepositoryBinding
	var raw, provider, origin string
	var revision, remoteID int64
	err := q.QueryRowContext(ctx, `SELECT revision,provider,api_origin,remote_id,CASE WHEN length(CAST(binding_json AS BLOB))<=4096 THEN binding_json ELSE '' END FROM platform_project_repositories WHERE project_id=?`, project).Scan(&revision, &provider, &origin, &remoteID, &raw)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if json.Unmarshal([]byte(raw), &p) != nil || revision <= 0 || p.Revision != revision || p.Provider != provider || p.APIOrigin != origin || p.RemoteID != remoteID {
		return RepositoryBinding{}, ErrConflict
	}
	valid, err := validateRepositoryBinding(p)
	if err != nil || valid.APIOrigin != p.APIOrigin {
		return RepositoryBinding{}, ErrConflict
	}
	return p, nil
}

func (s *Store) RepositoryBinding(ctx context.Context, project int, actor int64) (RepositoryBinding, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return RepositoryBinding{}, err
	}
	defer tx.Rollback()
	if _, err = requireProjectRole(ctx, tx, project, actor, "viewer"); err != nil {
		return RepositoryBinding{}, err
	}
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&exists); err != nil {
		return RepositoryBinding{}, err
	}
	p, err := readRepositoryBinding(ctx, tx, project)
	if err != nil {
		return RepositoryBinding{}, err
	}
	return p, tx.Commit()
}

func (s *Store) SaveRepositoryBinding(ctx context.Context, project int, actor, expected int64, p RepositoryBinding) (RepositoryBinding, error) {
	if project <= 0 || expected < 0 || expected == 1<<63-1 {
		return RepositoryBinding{}, ErrConflict
	}
	if _, err := validateRepositoryBinding(p); err != nil {
		return RepositoryBinding{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RepositoryBinding{}, err
	}
	defer tx.Rollback()
	out, err := saveRepositoryBindingTx(ctx, tx, project, actor, expected, p)
	if err != nil {
		return RepositoryBinding{}, err
	}
	if err = tx.Commit(); err != nil {
		return RepositoryBinding{}, err
	}
	return out, nil
}

func saveRepositoryBindingTx(ctx context.Context, tx *sql.Tx, project int, actor, expected int64, p RepositoryBinding) (RepositoryBinding, error) {
	if project <= 0 || expected < 0 || expected == 1<<63-1 {
		return RepositoryBinding{}, ErrConflict
	}
	p, err := validateRepositoryBinding(p)
	if err != nil {
		return RepositoryBinding{}, err
	}
	if _, err = requireProjectRole(ctx, tx, project, actor, "admin"); err != nil {
		return RepositoryBinding{}, err
	}
	var enabled bool
	if err = tx.QueryRowContext(ctx, `SELECT enabled FROM platform_projects WHERE id=?`, project).Scan(&enabled); err != nil {
		return RepositoryBinding{}, err
	}
	if !enabled {
		return RepositoryBinding{}, ErrConflict
	}
	current, err := readRepositoryBinding(ctx, tx, project)
	if err != nil {
		return RepositoryBinding{}, err
	}
	if current.Revision != expected {
		return RepositoryBinding{}, ErrConflict
	}
	var other int
	err = tx.QueryRowContext(ctx, `SELECT project_id FROM platform_project_repositories WHERE provider=? AND api_origin=? AND remote_id=? AND project_id<>?`, p.Provider, p.APIOrigin, p.RemoteID, project).Scan(&other)
	if err == nil {
		return RepositoryBinding{}, ErrConflict
	}
	if err != sql.ErrNoRows {
		return RepositoryBinding{}, err
	}
	integration, credentials, err := scanIntegration(tx.QueryRowContext(ctx, `SELECT `+integrationColumns+` FROM platform_integrations WHERE id=?`, p.IntegrationID))
	if err != nil {
		return RepositoryBinding{}, err
	}
	origin, err := canonicalRepositoryAPIOrigin(p.Provider, credentials.Endpoint)
	allowed := false
	for _, id := range integration.ProjectIDs {
		if id == project {
			allowed = true
		}
	}
	if err != nil || origin != p.APIOrigin || integration.Kind != p.Provider || !integration.Enabled || !allowed || strings.TrimSpace(credentials.Token) == "" {
		return RepositoryBinding{}, ErrConflict
	}
	p.Revision = expected + 1
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 4096 {
		return RepositoryBinding{}, ErrConflict
	}
	stamp := now()
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_project_repositories(project_id,revision,provider,api_origin,remote_id,binding_json,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(project_id) DO UPDATE SET revision=excluded.revision,provider=excluded.provider,api_origin=excluded.api_origin,remote_id=excluded.remote_id,binding_json=excluded.binding_json,updated_at=excluded.updated_at`, project, p.Revision, p.Provider, p.APIOrigin, p.RemoteID, string(raw), stamp); err != nil {
		return RepositoryBinding{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_repository_binding_history(project_id,revision,binding_json,actor,created_at) VALUES(?,?,?,?,?)`, project, p.Revision, string(raw), actor, stamp); err != nil {
		return RepositoryBinding{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'repository.binding.updated',?,?)`, actor, strconv.Itoa(project), stamp); err != nil {
		return RepositoryBinding{}, err
	}
	if err = markProjectSync(tx); err != nil {
		return RepositoryBinding{}, err
	}
	return p, nil
}
