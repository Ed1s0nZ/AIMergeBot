package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
)

type OwnerRouting struct {
	Revision     int64              `json:"revision"`
	DefaultOwner int64              `json:"default_owner"`
	Aliases      map[string][]int64 `json:"aliases"`
}

func migrateOwnerRouting(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS platform_owner_routing(project_id INTEGER PRIMARY KEY REFERENCES platform_projects(id),revision INTEGER NOT NULL,policy_json TEXT NOT NULL,updated_at TEXT NOT NULL);CREATE TABLE IF NOT EXISTS platform_owner_routing_history(project_id INTEGER NOT NULL REFERENCES platform_projects(id),revision INTEGER NOT NULL,policy_json TEXT NOT NULL,actor INTEGER NOT NULL REFERENCES platform_users(id),created_at TEXT NOT NULL,PRIMARY KEY(project_id,revision))`)
	return err
}
func readOwnerRouting(ctx context.Context, q queryRower, project int) (OwnerRouting, error) {
	p := OwnerRouting{Aliases: map[string][]int64{}}
	var raw string
	err := q.QueryRowContext(ctx, `SELECT revision,CASE WHEN length(CAST(policy_json AS BLOB))<=65536 THEN policy_json ELSE '' END FROM platform_owner_routing WHERE project_id=?`, project).Scan(&p.Revision, &raw)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return OwnerRouting{}, err
	}
	revision := p.Revision
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return OwnerRouting{}, ErrConflict
	}
	p.Revision = revision
	if p.Aliases == nil {
		p.Aliases = map[string][]int64{}
	}
	return p, nil
}
func (s *Store) OwnerRouting(ctx context.Context, project int, actor int64) (OwnerRouting, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return OwnerRouting{}, err
	}
	defer tx.Rollback()
	if _, err := requireProjectRole(ctx, tx, project, actor, "viewer"); err != nil {
		return OwnerRouting{}, err
	}
	var id int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&id); err != nil {
		return OwnerRouting{}, err
	}
	p, err := readOwnerRouting(ctx, tx, project)
	if err != nil {
		return OwnerRouting{}, err
	}
	return p, tx.Commit()
}
func validateOwnerRouting(p OwnerRouting) (map[int64]bool, error) {
	if p.DefaultOwner < 0 || len(p.Aliases) > 100 {
		return nil, ErrConflict
	}
	users := map[int64]bool{}
	if p.DefaultOwner > 0 {
		users[p.DefaultOwner] = true
	}
	for alias, ids := range p.Aliases {
		if len(alias) < 1 || len(alias) > 128 || strings.TrimSpace(alias) != alias || strings.ContainsAny(alias, "\r\n\t ") || len(ids) < 1 || len(ids) > 20 {
			return nil, ErrConflict
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if id <= 0 || seen[id] {
				return nil, ErrConflict
			}
			seen[id] = true
			users[id] = true
		}
	}
	return users, nil
}

func (s *Store) SaveOwnerRouting(ctx context.Context, project int, actor, expected int64, p OwnerRouting) (OwnerRouting, error) {
	if project <= 0 || expected < 0 || expected == 1<<63-1 {
		return OwnerRouting{}, ErrConflict
	}
	users, err := validateOwnerRouting(p)
	if err != nil {
		return OwnerRouting{}, err
	}
	if p.Aliases == nil {
		p.Aliases = map[string][]int64{}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return OwnerRouting{}, err
	}
	defer tx.Rollback()
	if _, err := requireProjectRole(ctx, tx, project, actor, "admin"); err != nil {
		return OwnerRouting{}, err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&exists); err != nil {
		return OwnerRouting{}, err
	}
	current, err := readOwnerRouting(ctx, tx, project)
	if err != nil {
		return OwnerRouting{}, err
	}
	if current.Revision != expected {
		return OwnerRouting{}, ErrConflict
	}
	for user := range users {
		if _, err := requireProjectRole(ctx, tx, project, user, "viewer"); err != nil {
			return OwnerRouting{}, err
		}
	}
	p.Revision = expected + 1
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 65536 {
		return OwnerRouting{}, ErrConflict
	}
	stamp := now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_owner_routing(project_id,revision,policy_json,updated_at) VALUES(?,?,?,?) ON CONFLICT(project_id) DO UPDATE SET revision=excluded.revision,policy_json=excluded.policy_json,updated_at=excluded.updated_at`, project, p.Revision, string(raw), stamp); err != nil {
		return OwnerRouting{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_owner_routing_history(project_id,revision,policy_json,actor,created_at) VALUES(?,?,?,?,?)`, project, p.Revision, string(raw), actor, stamp); err != nil {
		return OwnerRouting{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'owner.routing.updated',?,?)`, actor, strconv.Itoa(project), stamp); err != nil {
		return OwnerRouting{}, err
	}
	return p, tx.Commit()
}
