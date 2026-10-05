package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const associationResultBytes = 1024 * 1024

var ErrAssociationDecision = errors.New("invalid association decision")

func migrateFindingAssociations(tx *sql.Tx) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS platform_finding_association_history(id INTEGER PRIMARY KEY,current_run INTEGER NOT NULL REFERENCES platform_runs(id),current_finding TEXT NOT NULL,prior_run INTEGER NOT NULL REFERENCES platform_runs(id),prior_finding TEXT NOT NULL,association_id TEXT NOT NULL,decision TEXT NOT NULL CHECK(decision IN ('pending','confirmed','rejected')),reason TEXT NOT NULL,actor INTEGER NOT NULL,created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS platform_association_current ON platform_finding_association_history(current_run,association_id,id)`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Do not load tool traces or an unbounded historical result body for a hint.
func associationRun(ctx context.Context, tx *sql.Tx, id int64) (Run, bool, error) {
	r := Run{ID: id}
	var policy, result string
	var size int
	err := tx.QueryRowContext(ctx, `SELECT project_id,mr_iid,source_project_id,base_sha,head_sha,status,audit_policy_json,length(CAST(result_json AS BLOB)),CASE WHEN length(CAST(result_json AS BLOB))<=? THEN result_json ELSE '' END FROM platform_runs WHERE id=?`, associationResultBytes, id).Scan(&r.ProjectID, &r.MRIID, &r.SourceProjectID, &r.BaseSHA, &r.HeadSHA, &r.Status, &policy, &size, &result)
	if err != nil {
		return r, false, err
	}
	if err = json.Unmarshal([]byte(policy), &r.AuditPolicy); err != nil {
		return r, false, err
	}
	if size > associationResultBytes {
		return r, true, nil
	}
	if err = json.Unmarshal([]byte(result), &r.Result); err != nil {
		return r, false, err
	}
	return r, false, nil
}

func loadFindingAssociations(ctx context.Context, tx *sql.Tx, id, actor int64, role string) (FindingAssociations, error) {
	current, large, err := associationRun(ctx, tx, id)
	if err != nil {
		return FindingAssociations{}, err
	}
	if _, err = requireSnapshotRole(ctx, tx, current.Snapshot, actor, role); err != nil {
		return FindingAssociations{}, err
	}
	if large {
		return FindingAssociations{Items: []FindingAssociation{}, Truncated: true, Limitations: []string{"Current result exceeds the 1 MiB association input limit; inspect the original report manually."}}, nil
	}
	contextJSON, _ := json.Marshal(append([]ContextRepository{}, contextPolicyItems(current.Snapshot)...))
	source := current.SourceProjectID
	if source == 0 {
		source = current.ProjectID
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM platform_runs WHERE id<? AND project_id=? AND mr_iid=? AND COALESCE(NULLIF(source_project_id,0),project_id)=? AND COALESCE(json_extract(audit_policy_json,'$.context_repositories'),'[]')=? AND status NOT IN ('pending','running') ORDER BY id DESC LIMIT 21`, id, current.ProjectID, current.MRIID, source, string(contextJSON))
	if err != nil {
		return FindingAssociations{}, err
	}
	ids := []int64{}
	for rows.Next() {
		var prior int64
		if err = rows.Scan(&prior); err != nil {
			rows.Close()
			return FindingAssociations{}, err
		}
		ids = append(ids, prior)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return FindingAssociations{}, err
	}
	truncated := len(ids) > 20
	if truncated {
		ids = ids[:20]
	}
	priors := []Run{}
	skipped := false
	for _, priorID := range ids {
		prior, oversized, err := associationRun(ctx, tx, priorID)
		if err != nil {
			return FindingAssociations{}, err
		}
		if _, err = requireSnapshotRole(ctx, tx, prior.Snapshot, actor, "viewer"); err != nil {
			return FindingAssociations{}, err
		}
		if oversized {
			skipped = true
			continue
		}
		priors = append(priors, prior)
	}
	out := suggestFindingAssociations(current, priors)
	if truncated {
		out.Truncated = true
		out.Limitations = append(out.Limitations, "Only the latest 20 same-scope historical tasks were inspected.")
	}
	if skipped {
		out.Truncated = true
		out.Limitations = append(out.Limitations, "Historical results exceeding 1 MiB were omitted; inspect original reports manually.")
	}
	for i := range out.Items {
		item := &out.Items[i]
		rows, err := tx.QueryContext(ctx, `SELECT id,decision,reason,actor,created_at FROM platform_finding_association_history WHERE current_run=? AND association_id=? ORDER BY id DESC LIMIT 21`, id, item.ID)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var d FindingAssociationDecision
			if err = rows.Scan(&d.Revision, &d.Decision, &d.Reason, &d.Actor, &d.CreatedAt); err != nil {
				rows.Close()
				return out, err
			}
			item.History = append(item.History, d)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		if len(item.History) > 0 {
			item.State = item.History[0]
		}
		if len(item.History) > 20 {
			item.History = item.History[:20]
			item.HistoryTruncated = true
		}
	}
	return out, nil
}
func (s *Store) FindingAssociations(ctx context.Context, id, actor int64) (FindingAssociations, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return FindingAssociations{}, err
	}
	defer tx.Rollback()
	out, err := loadFindingAssociations(ctx, tx, id, actor, "viewer")
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

type AssociationDecisionRequest struct {
	Decision         string `json:"decision"`
	Reason           string `json:"reason"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func (s *Store) SaveFindingAssociation(ctx context.Context, id, actor int64, association string, req AssociationDecisionRequest) (FindingAssociationDecision, error) {
	var out FindingAssociationDecision
	if (req.Decision != "pending" && req.Decision != "confirmed" && req.Decision != "rejected") || req.ExpectedRevision < 0 || len(req.Reason) > 4000 || utf8.RuneCountInString(req.Reason) > 1000 || !utf8.ValidString(req.Reason) || strings.ContainsRune(req.Reason, 0) || (req.Decision != "pending" && strings.TrimSpace(req.Reason) == "") {
		return out, ErrAssociationDecision
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	items, err := loadFindingAssociations(ctx, tx, id, actor, "reviewer")
	if err != nil {
		return out, err
	}
	var candidate *FindingAssociation
	for i := range items.Items {
		if items.Items[i].ID == association {
			candidate = &items.Items[i]
			break
		}
	}
	if candidate == nil {
		return out, ErrConflict
	}
	if candidate.State.Revision != req.ExpectedRevision {
		return out, ErrConflict
	}
	if req.Decision == "confirmed" {
		var conflicts int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_finding_association_history h WHERE h.current_run=? AND h.decision='confirmed' AND h.association_id<>? AND (h.current_finding=? OR (h.prior_run=? AND h.prior_finding=?)) AND h.id=(SELECT MAX(n.id) FROM platform_finding_association_history n WHERE n.current_run=h.current_run AND n.association_id=h.association_id)`, id, association, candidate.FindingID, candidate.PriorRunID, candidate.PriorFindingID).Scan(&conflicts)
		if err != nil {
			return out, err
		}
		if conflicts > 0 {
			return out, ErrConflict
		}
	}
	out = FindingAssociationDecision{Decision: req.Decision, Reason: strings.TrimSpace(req.Reason), Actor: actor, CreatedAt: now()}
	res, err := tx.ExecContext(ctx, `INSERT INTO platform_finding_association_history(current_run,current_finding,prior_run,prior_finding,association_id,decision,reason,actor,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, candidate.FindingID, candidate.PriorRunID, candidate.PriorFindingID, association, out.Decision, out.Reason, actor, out.CreatedAt)
	if err != nil {
		return out, err
	}
	out.Revision, err = res.LastInsertId()
	if err != nil {
		return out, err
	}
	event, _ := json.Marshal(struct {
		Run         int64  `json:"run"`
		Association string `json:"association"`
		Revision    int64  `json:"revision"`
		Decision    string `json:"decision"`
	}{id, association, out.Revision, out.Decision})
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_events(actor,action,target,created_at) VALUES(?,'finding.association_decided',?,?)`, actor, string(event), out.CreatedAt); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
