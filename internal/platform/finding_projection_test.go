package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFindingProjectionTracksResultAndRollback(t *testing.T) {
	s := testStore(t)
	id, _, err := s.Enqueue(context.Background(), Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	result := `{"findings":[{"id":"f1","severity":"high","type":"access"},{"id":"f2","severity":"low","type":"data"}]}`
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json=? WHERE id=?`, result, id); err != nil {
		t.Fatal(err)
	}
	var count int
	check := func(want int) {
		t.Helper()
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM platform_finding_index WHERE run_id=?`, id).Scan(&count); err != nil || count != want {
			t.Fatalf("projection count %d want %d: %v", count, want, err)
		}
	}
	check(2)
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE platform_runs SET result_json='{"findings":[]}' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	check(2)
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='malformed' WHERE id=?`, id); err == nil {
		t.Fatal("malformed result accepted")
	}
	check(2)
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[]}' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	check(0)
}

func TestFindingProjectionBackfillRunsOnlyOnce(t *testing.T) {
	s := testStore(t)
	id, _, err := s.Enqueue(context.Background(), Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json='{"findings":[{"id":"old","severity":"medium","type":"legacy"}]}' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-projection database for the one-time migration.
	for _, q := range []string{`DROP TRIGGER platform_finding_insert`, `DROP TRIGGER platform_finding_update`, `DROP TRIGGER platform_finding_delete`, `DROP TABLE platform_finding_index`, `DELETE FROM platform_projection_schema WHERE name='findings'`} {
		if _, err = s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	var finding string
	if err = s.DB.QueryRow(`SELECT finding_id FROM platform_finding_index WHERE run_id=?`, id).Scan(&finding); err != nil || finding != "old" {
		t.Fatalf("backfill %q: %v", finding, err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM platform_finding_index WHERE run_id=?`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("repeated migration %d: %v", count, err)
	}
}

func TestRunListPayloadIndependentOfEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, _, err := s.Enqueue(ctx, Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: "base", HeadSHA: "head"}, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("private-source", 200000)
	raw, err := json.Marshal(AuditResult{Findings: []Finding{{ID: "f1", Severity: "high", Type: "access", Evidence: huge}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE platform_runs SET result_json=?,trace_json='invalid trace',audit_policy_json='invalid policy' WHERE id=?`, string(raw), id); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.listRuns(ctx, " WHERE 1=1", nil, 1, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].FindingCount != 1 {
		t.Fatalf("list %+v total%d: %v", items, total, err)
	}
	body, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > 2000 || strings.Contains(string(body), "private-source") || strings.Contains(string(body), `"result"`) {
		t.Fatalf("list payload grew to %d", len(body))
	}
	if _, err = s.Run(ctx, id); err == nil {
		t.Fatal("detail must decode invalid fixture policy/trace")
	}
	// A historical review for a removed finding must not satisfy a current filter.
	if _, err = s.DB.Exec(`INSERT INTO platform_reviews(run_id,finding_id,status,reason,actor,updated_at) VALUES(?,'gone','fixed','',1,'')`, id); err != nil {
		t.Fatal(err)
	}
	_, total, err = s.listRuns(ctx, ` WHERE EXISTS(SELECT 1 FROM platform_finding_index f JOIN platform_reviews r ON r.run_id=f.run_id AND r.finding_id=f.finding_id WHERE f.run_id=platform_runs.id AND r.status=?)`, []any{"fixed"}, 1, 20)
	if err != nil || total != 0 {
		t.Fatalf("stale review matched total%d: %v", total, err)
	}
}

func TestFindingProjectionFilterUsesCoveringIndex(t *testing.T) {
	s := testStore(t)
	rows, err := s.DB.Query(`EXPLAIN QUERY PLAN SELECT 1 FROM platform_finding_index WHERE run_id=1 AND severity='high'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "COVERING INDEX platform_finding_severity") {
			found = true
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("severity query did not use covering index")
	}
	if strings.Contains(runListColumns, "result_json") || strings.Contains(runListColumns, "trace_json") || strings.Contains(runListColumns, "audit_policy_json") {
		t.Fatal("list selects full payload")
	}
}
