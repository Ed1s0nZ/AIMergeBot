package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrWorkflowPolicy = errors.New("invalid workflow policy")

type WorkflowPolicy struct {
	Revision           int64    `json:"revision"`
	Focus              []string `json:"focus"`
	ExcludedExtensions []string `json:"excluded_extensions"`
}

func migrateWorkflowPolicy(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_workflow_policies(project_id INTEGER PRIMARY KEY REFERENCES platform_projects(id),revision INTEGER NOT NULL,policy_json TEXT NOT NULL,updated_at TEXT NOT NULL)`)
	return err
}

func readWorkflowPolicy(ctx context.Context, q queryRower, project int) (WorkflowPolicy, error) {
	p := WorkflowPolicy{Focus: []string{}, ExcludedExtensions: []string{}}
	var raw string
	err := q.QueryRowContext(ctx, `SELECT revision,policy_json FROM platform_workflow_policies WHERE project_id=?`, project).Scan(&p.Revision, &raw)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	revision := p.Revision
	err = json.Unmarshal([]byte(raw), &p)
	p.Revision = revision
	return p, err
}

func validateWorkflowPolicy(p WorkflowPolicy) error {
	allowed := map[string]bool{"authorization": true, "injection": true, "secrets": true, "path_traversal": true, "ssrf": true, "deserialization": true, "dependencies": true, "business_logic": true}
	if len(p.Focus) > len(allowed) || len(p.ExcludedExtensions) > 50 {
		return ErrWorkflowPolicy
	}
	seen := map[string]bool{}
	for _, v := range p.Focus {
		if !allowed[v] || seen[v] {
			return ErrWorkflowPolicy
		}
		seen[v] = true
	}
	seen = map[string]bool{}
	for _, v := range p.ExcludedExtensions {
		if len(v) < 2 || len(v) > 32 || v[0] != '.' || seen[v] {
			return ErrWorkflowPolicy
		}
		for _, c := range v[1:] {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return ErrWorkflowPolicy
			}
		}
		seen[v] = true
	}
	return nil
}

func (s *Store) SaveWorkflowPolicy(ctx context.Context, project int, actor int64, expected int64, p WorkflowPolicy) (WorkflowPolicy, error) {
	if project <= 0 || expected < 0 || validateWorkflowPolicy(p) != nil {
		return p, ErrWorkflowPolicy
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	if _, err = requireProjectRole(ctx, tx, project, actor, "admin"); err != nil {
		return p, err
	}
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&exists); err != nil {
		return p, err
	}
	current, err := readWorkflowPolicy(ctx, tx, project)
	if err != nil {
		return p, err
	}
	if current.Revision != expected {
		return current, ErrConflict
	}
	p.Revision = expected + 1
	if p.Focus == nil {
		p.Focus = []string{}
	}
	if p.ExcludedExtensions == nil {
		p.ExcludedExtensions = []string{}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_workflow_policies(project_id,revision,policy_json,updated_at) VALUES(?,?,?,?) ON CONFLICT(project_id) DO UPDATE SET revision=excluded.revision,policy_json=excluded.policy_json,updated_at=excluded.updated_at`, project, p.Revision, string(raw), now())
	if err != nil {
		return p, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'workflow_policy.saved',?,?)`, actor, project, now())
	if err != nil {
		return p, err
	}
	return p, tx.Commit()
}
