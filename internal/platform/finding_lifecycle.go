package platform

import (
	"context"
)

type FindingOccurrence struct {
	RunID     int64  `json:"run_id"`
	FindingID string `json:"finding_id"`
	HeadSHA   string `json:"head_sha"`
	RunStatus string `json:"run_status"`
}
type FindingReviewReceipt struct {
	RunID     int64  `json:"run_id"`
	FindingID string `json:"finding_id"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Actor     int64  `json:"actor"`
	CreatedAt string `json:"created_at"`
	Imported  bool   `json:"imported"`
}
type FindingHistory struct {
	FindingID            string                 `json:"finding_id"`
	Fingerprint          string                 `json:"fingerprint"`
	FirstRunID           int64                  `json:"first_run_id"`
	LastRunID            int64                  `json:"last_run_id"`
	Occurrences          []FindingOccurrence    `json:"occurrences"`
	Reviews              []FindingReviewReceipt `json:"reviews"`
	OccurrencesTruncated bool                   `json:"occurrences_truncated"`
	ReviewsTruncated     bool                   `json:"reviews_truncated"`
}
type FindingLifecycle struct {
	HistoryTruncated       bool                `json:"history_truncated"`
	Current                []FindingHistory    `json:"current"`
	NotReobserved          []FindingOccurrence `json:"not_reobserved"`
	NotReobservedTruncated bool                `json:"not_reobserved_truncated"`
}

// Called only after current snapshot authorization. SQL additionally constrains
// every historic row to the same target/MR/source, and never scans result bodies.
func (s *Store) FindingLifecycle(ctx context.Context, run Run) (FindingLifecycle, error) {
	out := FindingLifecycle{Current: []FindingHistory{}, NotReobserved: []FindingOccurrence{}}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	scope := `r.project_id=? AND r.mr_iid=? AND COALESCE(NULLIF(r.source_project_id,0),r.project_id)=? AND r.id<=?`
	source := run.SourceProjectID
	if source == 0 {
		source = run.ProjectID
	}
	args := []any{run.ID, run.ProjectID, run.MRIID, source, run.ID}
	rows, err := tx.QueryContext(ctx, `WITH matched AS (
 SELECT c.finding_id current_id,c.fingerprint,o.run_id,o.finding_id,r.head_sha,r.status,
 ROW_NUMBER() OVER(PARTITION BY c.finding_id ORDER BY o.run_id DESC) rank,
 COUNT(*) OVER(PARTITION BY c.finding_id) total,
 MIN(o.run_id) OVER(PARTITION BY c.finding_id) first_run,
 MAX(o.run_id) OVER(PARTITION BY c.finding_id) last_run
 FROM platform_finding_occurrences c JOIN platform_finding_occurrences o ON o.fingerprint=c.fingerprint
 JOIN platform_runs r ON r.id=o.run_id WHERE c.run_id=? AND `+scope+`)
 SELECT current_id,fingerprint,run_id,finding_id,head_sha,status,total,first_run,last_run FROM matched WHERE rank<=20 ORDER BY current_id,rank LIMIT 1001`, args...)
	if err != nil {
		return out, err
	}
	index := map[string]int{}
	occurrenceCount := 0
	for rows.Next() {
		occurrenceCount++
		if occurrenceCount > 1000 {
			out.HistoryTruncated = true
			break
		}
		var current, fp string
		var occurrence FindingOccurrence
		var total int
		var first, last int64
		if err = rows.Scan(&current, &fp, &occurrence.RunID, &occurrence.FindingID, &occurrence.HeadSHA, &occurrence.RunStatus, &total, &first, &last); err != nil {
			rows.Close()
			return out, err
		}
		i, ok := index[current]
		if !ok {
			i = len(out.Current)
			index[current] = i
			out.Current = append(out.Current, FindingHistory{FindingID: current, Fingerprint: fp, FirstRunID: first, LastRunID: last, Occurrences: []FindingOccurrence{}, Reviews: []FindingReviewReceipt{}, OccurrencesTruncated: total > 20})
		}
		out.Current[i].Occurrences = append(out.Current[i].Occurrences, occurrence)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.QueryContext(ctx, `WITH receipts AS (
 SELECT c.finding_id current_id,h.run_id,h.finding_id,h.status,h.reason,h.actor,h.created_at,h.imported,
 ROW_NUMBER() OVER(PARTITION BY c.finding_id ORDER BY h.id DESC) rank,COUNT(*) OVER(PARTITION BY c.finding_id) total
 FROM platform_finding_occurrences c JOIN platform_finding_occurrences o ON o.fingerprint=c.fingerprint
 JOIN platform_runs r ON r.id=o.run_id JOIN platform_review_history h ON h.run_id=o.run_id AND h.finding_id=o.finding_id
 WHERE c.run_id=? AND `+scope+`)
 SELECT current_id,run_id,finding_id,status,reason,actor,created_at,imported,total FROM receipts WHERE rank<=20 ORDER BY current_id,rank LIMIT 201`, args...)
	if err != nil {
		return out, err
	}
	receiptCount := 0
	for rows.Next() {
		receiptCount++
		if receiptCount > 200 {
			out.HistoryTruncated = true
			break
		}
		var current string
		var receipt FindingReviewReceipt
		var total int
		if err = rows.Scan(&current, &receipt.RunID, &receipt.FindingID, &receipt.Status, &receipt.Reason, &receipt.Actor, &receipt.CreatedAt, &receipt.Imported, &total); err != nil {
			rows.Close()
			return out, err
		}
		if i, ok := index[current]; ok {
			out.Current[i].Reviews = append(out.Current[i].Reviews, receipt)
			out.Current[i].ReviewsTruncated = total > 20
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.QueryContext(ctx, `WITH absent AS (
 SELECT o.run_id,o.finding_id,r.head_sha,r.status,ROW_NUMBER() OVER(PARTITION BY o.fingerprint ORDER BY o.run_id DESC) rank
 FROM platform_finding_occurrences o JOIN platform_runs r ON r.id=o.run_id
 WHERE `+scope+` AND r.id<? AND NOT EXISTS(SELECT 1 FROM platform_finding_occurrences c WHERE c.run_id=? AND c.fingerprint=o.fingerprint))
 SELECT run_id,finding_id,head_sha,status FROM absent WHERE rank=1 ORDER BY run_id DESC,finding_id LIMIT 51`, run.ProjectID, run.MRIID, source, run.ID, run.ID, run.ID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var occurrence FindingOccurrence
		if err = rows.Scan(&occurrence.RunID, &occurrence.FindingID, &occurrence.HeadSHA, &occurrence.RunStatus); err != nil {
			rows.Close()
			return out, err
		}
		out.NotReobserved = append(out.NotReobserved, occurrence)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.NotReobserved) > 50 {
		out.NotReobserved = out.NotReobserved[:50]
		out.NotReobservedTruncated = true
	}
	return out, tx.Commit()
}
