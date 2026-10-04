package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	legacy "pr_agent/internal"
)

// ImportLegacy preserves original tables and makes the missing commit evidence explicit.
func (s *Store) ImportLegacy(ctx context.Context) error {
	var exists int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='results'`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,result_json FROM results ORDER BY id`)
	if err != nil {
		return err
	}
	type entry struct {
		id  int64
		raw string
	}
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.id, &e.raw); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		var old legacy.MRAnalysisResult
		if err = json.Unmarshal([]byte(e.raw), &old); err != nil {
			return fmt.Errorf("legacy result %d invalid JSON: %w", e.id, err)
		}
		findings := []Finding{}
		for i, f := range old.Result {
			findings = append(findings, Finding{Type: f.Type, ID: fmt.Sprintf("legacy-%d-%d", e.id, i), File: f.File, Severity: f.Level, Title: f.Type, Description: f.Desc, Evidence: string(f.Code), Suggestion: f.Suggestion, Confidence: "candidate", Trigger: "Legacy record; no pinned commit or verified line"})
		}
		result := AuditResult{Findings: findings, Summary: "Imported legacy audit; submission evidence was not recorded", CoverageNotes: []string{"Legacy result has no base/head SHA or verified line; review original evidence before relying on it"}}
		raw, _ := json.Marshal(result)
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		var count int
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_legacy_imports WHERE legacy_id=?`, e.id).Scan(&count)
		if err != nil {
			tx.Rollback()
			return err
		}
		if count > 0 {
			tx.Rollback()
			continue
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO platform_runs(project_id,mr_iid,source_project_id,base_sha,head_sha,title,url,status,result_json,created_at,finished_at,requested_by,policy_version) VALUES(?,?,?,'','',?,?,'incomplete',?,?,?,0,'legacy')`, old.ProjectID, old.MRID, old.ProjectID, old.MRTitle, old.MRUrl, string(raw), now(), time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			tx.Rollback()
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_legacy_imports(legacy_id,run_id) VALUES(?,?)`, e.id, id); err != nil {
			tx.Rollback()
			return err
		}
		for i, f := range old.Result {
			status := f.ReviewStatus
			if status == "" || status == "pending" {
				continue
			}
			switch status {
			case "accepted", "false_positive", "fixed":
			default:
				status = "pending"
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO platform_reviews(run_id,finding_id,status,reason,actor,updated_at) VALUES(?,?,?,'Imported legacy finding review',0,?)`, id, findings[i].ID, status, now()); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		if old.ProjectID > 0 {
			if _, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO platform_projects(id,name,enabled) VALUES(?,?,1)`, old.ProjectID, old.ProjectName); err != nil {
				return err
			}
		}
	}
	return nil
}
