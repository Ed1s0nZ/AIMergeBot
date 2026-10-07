package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrProjectPermission = errors.New("project permission required")

type ProjectPermissions struct {
	Role      string `json:"role"`
	CanSubmit bool   `json:"can_submit"`
	CanReview bool   `json:"can_review"`
	CanCancel bool   `json:"can_cancel"`
}

func roleRank(role string) int {
	switch role {
	case "viewer":
		return 1
	case "reviewer":
		return 2
	case "operator":
		return 3
	case "admin":
		return 4
	}
	return 0
}

func permissions(role string) ProjectPermissions {
	return ProjectPermissions{Role: role, CanSubmit: roleRank(role) >= 3, CanReview: roleRank(role) >= 2, CanCancel: roleRank(role) >= 3}
}

func migrateProjectAccess(tx *sql.Tx) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS platform_project_members(project_id INTEGER NOT NULL REFERENCES platform_projects(id),user_id INTEGER NOT NULL REFERENCES platform_users(id),role TEXT NOT NULL CHECK(role IN ('viewer','reviewer','operator')),PRIMARY KEY(project_id,user_id))`,
		`CREATE INDEX IF NOT EXISTS platform_member_projects ON platform_project_members(user_id,project_id)`,
		`CREATE INDEX IF NOT EXISTS platform_project_actor_runs ON platform_runs(project_id,requested_by,status)`,
		`CREATE INDEX IF NOT EXISTS platform_source_actor_runs ON platform_runs(source_project_id,requested_by,status)`,
		`CREATE TABLE IF NOT EXISTS platform_access_migrations(name TEXT PRIMARY KEY)`,
		`INSERT OR IGNORE INTO platform_project_members(project_id,user_id,role) SELECT p.id,u.id,'operator' FROM platform_projects p CROSS JOIN platform_users u WHERE u.role='member' AND NOT EXISTS(SELECT 1 FROM platform_access_migrations WHERE name='project-members-v1')`,
		`INSERT OR IGNORE INTO platform_access_migrations(name) VALUES('project-members-v1')`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func projectRole(ctx context.Context, q queryRower, project int, user int64) (string, error) {
	var role string
	var disabled bool
	if err := q.QueryRowContext(ctx, `SELECT role,disabled FROM platform_users WHERE id=?`, user).Scan(&role, &disabled); err != nil {
		return "", err
	}
	if disabled {
		return "", ErrCredentials
	}
	if role == "admin" {
		return role, nil
	}
	err := q.QueryRowContext(ctx, `SELECT role FROM platform_project_members WHERE project_id=? AND user_id=?`, project, user).Scan(&role)
	return role, err
}

func requireProjectRole(ctx context.Context, q queryRower, project int, user int64, required string) (string, error) {
	role, err := projectRole(ctx, q, project, user)
	if err != nil {
		return "", err
	}
	if roleRank(role) < roleRank(required) {
		return role, ErrProjectPermission
	}
	return role, nil
}

func requireSnapshotRole(ctx context.Context, q queryRower, snap Snapshot, user int64, required string) (string, error) {
	role, err := requireProjectRole(ctx, q, snap.ProjectID, user, required)
	if err != nil {
		return "", err
	}
	if role != "admin" && snap.SourceProjectID != snap.ProjectID {
		if _, err = requireProjectRole(ctx, q, snap.SourceProjectID, user, "viewer"); err != nil {
			return "", err
		}
	}
	if err = requireContextRoles(ctx, q, snap, user); err != nil {
		return "", err
	}
	return role, nil
}

func (s *Store) ProjectsForUser(ctx context.Context, user User) ([]Project, error) {
	if user.Role == "admin" {
		items, err := s.Projects(ctx)
		for i := range items {
			items[i].AccessRole = "admin"
		}
		return items, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id,p.name,p.enabled,m.role FROM platform_projects p JOIN platform_project_members m ON m.project_id=p.id WHERE m.user_id=? ORDER BY p.name,p.id`, user.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Project{}
	for rows.Next() {
		var p Project
		if err = rows.Scan(&p.ID, &p.Name, &p.Enabled, &p.AccessRole); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

type ProjectMember struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

func (s *Store) ProjectMembers(ctx context.Context, project int) ([]ProjectMember, error) {
	var exists int
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&exists); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT u.id,u.username,m.role,u.disabled FROM platform_project_members m JOIN platform_users u ON u.id=m.user_id WHERE m.project_id=? AND u.role='member' ORDER BY u.username`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ProjectMember{}
	for rows.Next() {
		var m ProjectMember
		if err = rows.Scan(&m.UserID, &m.Username, &m.Role, &m.Disabled); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

func (s *Store) SetProjectMember(ctx context.Context, project int, user int64, role string, actor int64) error {
	if role != "" && (roleRank(role) == 0 || role == "admin") {
		return ErrProjectPermission
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var actorRole string
	var disabled bool
	if err = tx.QueryRowContext(ctx, `SELECT role,disabled FROM platform_users WHERE id=?`, actor).Scan(&actorRole, &disabled); err != nil {
		return err
	}
	if actorRole != "admin" || disabled {
		return ErrProjectPermission
	}
	var id int
	if err = tx.QueryRowContext(ctx, `SELECT id FROM platform_projects WHERE id=?`, project).Scan(&id); err != nil {
		return err
	}
	var userRole string
	if err = tx.QueryRowContext(ctx, `SELECT role FROM platform_users WHERE id=?`, user).Scan(&userRole); err != nil {
		return err
	}
	if userRole != "member" {
		return ErrProjectPermission
	}
	if role == "" {
		if err = cancelRevokedContextMember(ctx, tx, project, user); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM platform_project_members WHERE project_id=? AND user_id=?`, project, user)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO platform_project_members(project_id,user_id,role) VALUES(?,?,?) ON CONFLICT(project_id,user_id) DO UPDATE SET role=excluded.role`, project, user, role)
	}
	if err != nil {
		return err
	}
	if role != "operator" {
		if _, err = tx.ExecContext(ctx, `UPDATE platform_runs SET status='cancelled',error='project execution permission revoked',finished_at=? WHERE requested_by=? AND status IN ('pending','running') AND (project_id=? OR (source_project_id=? AND ?=1))`, now(), user, project, project, role == ""); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'project.access.changed',?,?)`, actor, fmt.Sprintf("project=%d user=%d role=%s", project, user, role), now()); err != nil {
		return err
	}
	return tx.Commit()
}
