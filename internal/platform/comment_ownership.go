package platform

import (
	"context"
)

// A finished attempt may publish only through the current service instance.
// Empty-owner legacy primitives are unavailable while a live instance exists.
const commentOwnerPredicate = ` AND worker_owner=? AND (
 (worker_owner='' AND NOT EXISTS(SELECT 1 FROM platform_worker_instance i WHERE julianday(i.lease_until)>julianday('now')))
 OR (worker_owner<>'' AND EXISTS(SELECT 1 FROM platform_worker_instance i WHERE i.id=1 AND i.owner=platform_runs.worker_owner AND julianday(i.lease_until)>julianday('now'))))`

func (s *Store) claimOwnedComment(ctx context.Context, id int64, owner string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO platform_comments(run_id,status,updated_at) SELECT id,'sending',? FROM platform_runs WHERE id=? AND status='succeeded'`+commentOwnerPredicate, now(), id, owner)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (s *Store) commentOwnerValid(ctx context.Context, id int64, owner string) bool {
	var valid bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM platform_runs WHERE id=? AND status='succeeded'`+commentOwnerPredicate+`)`, id, owner).Scan(&valid)
	return err == nil && valid
}
func (s *Store) finishOwnedComment(ctx context.Context, id int64, owner, status string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE platform_comments SET status=?,updated_at=? WHERE run_id=? AND EXISTS(SELECT 1 FROM platform_runs WHERE id=? AND status='succeeded'`+commentOwnerPredicate+`)`, status, now(), id, id, owner)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
