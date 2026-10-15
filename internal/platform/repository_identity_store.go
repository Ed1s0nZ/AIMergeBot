package platform

import (
	"context"
	"database/sql"
)

type repositoryIdentity struct {
	Kind     string
	Revision int64
	Health   string
}

// Historical legacy runs can reference projects absent from today's catalog.
// Reusing those IDs would attach their reports to a new repository's ACL.
func nextRepositoryProjectID(ctx context.Context, tx *sql.Tx) (int, error) {
	var raw any
	err := tx.QueryRowContext(ctx, `SELECT MAX(reserved_id) FROM (
 SELECT COALESCE(MAX(id),0) AS reserved_id FROM platform_projects
 UNION ALL SELECT MAX(project_id) FROM platform_repository_identities
 UNION ALL SELECT MAX(project_id) FROM platform_runs
 UNION ALL SELECT MAX(source_project_id) FROM platform_runs
 UNION ALL SELECT MAX(project_id) FROM platform_run_context_repositories
)`).Scan(&raw)
	if err != nil {
		return 0, err
	}
	highest, ok := raw.(int64)
	if !ok || highest < 0 || highest == 1<<63-1 || int64(int(highest+1)) != highest+1 {
		return 0, ErrConflict
	}
	return int(highest + 1), nil
}

func readRepositoryIdentity(ctx context.Context, q queryRower, project int) (repositoryIdentity, error) {
	var r repositoryIdentity
	var kindType, revisionType, healthType string
	err := q.QueryRowContext(ctx, `SELECT kind,revision,health,typeof(kind),typeof(revision),typeof(health) FROM platform_repository_identities WHERE project_id=?`, project).Scan(&r.Kind, &r.Revision, &r.Health, &kindType, &revisionType, &healthType)
	if err != nil || kindType != "text" || revisionType != "integer" || healthType != "text" || r.Revision <= 0 || (r.Kind != "legacy" && r.Kind != "internal") || (r.Health != "valid" && r.Health != "repair_required") {
		return repositoryIdentity{}, ErrRepositoryUnavailable
	}
	return r, nil
}

// Existing projects never regain legacy identity merely because a row is lost.
func ensureProjectIdentityTx(ctx context.Context, tx *sql.Tx, project int, existed bool, kind string) error {
	if existed {
		_, err := readRepositoryIdentity(ctx, tx, project)
		return err
	}
	if kind != "legacy" && kind != "internal" {
		return ErrConflict
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO platform_repository_identities(project_id,kind,revision,health,created_at,updated_at) VALUES(?,?,1,'valid',?,?)`, project, kind, now(), now())
	return err
}

func advanceRepositoryIdentityTx(ctx context.Context, tx *sql.Tx, project int, current repositoryIdentity, initial bool) error {
	if initial {
		if current.Kind != "internal" || current.Revision != 1 || current.Health != "valid" {
			return ErrConflict
		}
		return nil
	}
	if current.Health != "valid" || current.Revision == 1<<63-1 {
		return ErrConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE platform_repository_identities SET kind='internal',revision=revision+1,updated_at=? WHERE project_id=? AND kind=? AND revision=? AND health='valid'`, now(), project, current.Kind, current.Revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}

// i.id is supplied by a bounded project-id CTE in the two legacy guards.
const legacyRepositoryUnavailablePredicate = `
 EXISTS(SELECT 1 FROM platform_project_repositories WHERE project_id=i.id)
 OR EXISTS(SELECT 1 FROM platform_repository_binding_history WHERE project_id=i.id)
 OR EXISTS(SELECT 1 FROM platform_repository_project_receipts WHERE project_id=i.id)
 OR EXISTS(SELECT 1 FROM platform_repository_identities r WHERE r.project_id=i.id AND NOT(r.kind='legacy' AND r.health='valid' AND typeof(r.revision)='integer' AND r.revision>0))
 OR EXISTS(SELECT 1 FROM platform_projects p WHERE p.id=i.id AND NOT EXISTS(SELECT 1 FROM platform_repository_identities r WHERE r.project_id=p.id AND r.kind='legacy' AND r.health='valid' AND typeof(r.revision)='integer' AND r.revision>0))`
