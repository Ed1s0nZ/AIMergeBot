package platform

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func lifecycleRun(t *testing.T, s *Store, snap Snapshot, findings []Finding, status string) Run {
	t.Helper()
	ctx := context.Background()
	for i := range findings {
		findings[i].Fingerprint = findingFingerprint(snap, findings[i])
	}
	id, _, err := s.Enqueue(ctx, snap, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, id, status, "", AuditResult{Findings: findings, Summary: "synthetic lifecycle audit", CoverageNotes: []string{}}, nil); err != nil {
		t.Fatal(err)
	}
	run, err := s.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestFindingLifecycleRetainsHistoryWithoutCopyingDecisionOrCertifyingFix(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	f := Finding{ID: "first", File: "file.any", Side: "head", Line: 2, Type: "risk", Evidence: "danger(input)", Trigger: "untrusted input"}
	first := lifecycleRun(t, s, snap, []Finding{f}, "succeeded")
	if err := s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: first.ID, FindingID: f.ID, Status: "fixed", Reason: "human conclusion on old version", Actor: 1})); err != nil {
		t.Fatal(err)
	}
	snap.HeadSHA = strings.Repeat("c", 40)
	f.ID = "second"
	f.Line = 20
	f.Title = "rephrased"
	second := lifecycleRun(t, s, snap, []Finding{f}, "succeeded")
	history, err := s.FindingLifecycle(ctx, second)
	if err != nil || len(history.Current) != 1 || history.Current[0].FirstRunID != first.ID || len(history.Current[0].Occurrences) != 2 || len(history.Current[0].Reviews) != 1 {
		t.Fatal("missing recurring history", history, err)
	}
	reviews, err := s.Reviews(ctx, second.ID)
	if err != nil || len(reviews) != 0 {
		t.Fatal("old fixed decision inherited", reviews, err)
	}
	snap.HeadSHA = strings.Repeat("d", 40)
	absent := lifecycleRun(t, s, snap, []Finding{}, "incomplete")
	history, err = s.FindingLifecycle(ctx, absent)
	if err != nil || len(history.Current) != 0 || len(history.NotReobserved) != 1 || history.NotReobserved[0].RunID != second.ID {
		t.Fatal("absence misrepresented or lost", history, err)
	}
}

func TestFindingLifecycleHistoryBoundsAndScopeDefense(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	f := Finding{ID: "first", File: "file.any", Side: "head", Type: "risk", Evidence: "danger(input)", Trigger: "untrusted input"}
	first := lifecycleRun(t, s, snap, []Finding{f}, "succeeded")
	for i := 0; i < 23; i++ {
		if err := s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: first.ID, FindingID: f.ID, Status: "pending", Reason: "bounded receipt", Actor: 1})); err != nil {
			t.Fatal(err)
		}
	}
	history, err := s.FindingLifecycle(ctx, first)
	if err != nil || len(history.Current) != 1 || len(history.Current[0].Reviews) != 20 || !history.Current[0].ReviewsTruncated {
		t.Fatal("unbounded review history", history, err)
	}
	snap.SourceProjectID = 2
	snap.HeadSHA = strings.Repeat("c", 40)
	f.ID = "other-source"
	other := lifecycleRun(t, s, snap, []Finding{f}, "succeeded")
	// Even a corrupted occurrence hash cannot bypass SQL source-scope defense.
	if _, err = s.DB.Exec(`UPDATE platform_finding_occurrences SET fingerprint=? WHERE run_id=?`, history.Current[0].Fingerprint, other.ID); err != nil {
		t.Fatal(err)
	}
	history, err = s.FindingLifecycle(ctx, other)
	if err != nil || len(history.Current) != 1 || len(history.Current[0].Occurrences) != 1 || len(history.Current[0].Reviews) != 0 || len(history.NotReobserved) != 0 {
		t.Fatal("cross-source history exposed", history, err)
	}
}

func TestFindingLifecycleTotalResponseBudget(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	findings := []Finding{}
	for i := 0; i < 11; i++ {
		name := string(rune('a' + i))
		findings = append(findings, Finding{ID: name, File: name + ".any", Side: "head", Type: "risk", Evidence: "danger(input)", Trigger: "input"})
	}
	run := lifecycleRun(t, s, snap, findings, "succeeded")
	for _, f := range findings {
		for i := 0; i < 20; i++ {
			if err := s.SaveReview(ctx, reviewAtCurrentRevision(t, s, ctx, Review{RunID: run.ID, FindingID: f.ID, Status: "pending", Reason: "bounded receipt", Actor: 1})); err != nil {
				t.Fatal(err)
			}
		}
	}
	history, err := s.FindingLifecycle(ctx, run)
	total := 0
	for _, f := range history.Current {
		total += len(f.Reviews)
	}
	if err != nil || !history.HistoryTruncated || total != 200 {
		t.Fatal("response receipt budget bypass", total, history.HistoryTruncated, err)
	}
}

func TestFindingLifecycleOccurrenceAndPriorOnlyCaps(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	snap := Snapshot{ProjectID: 1, SourceProjectID: 1, MRIID: 1, BaseSHA: strings.Repeat("a", 40)}
	f := Finding{ID: "same", File: "file.any", Side: "head", Type: "risk", Evidence: "danger(input)", Trigger: "input"}
	var latest Run
	for i := 1; i <= 22; i++ {
		snap.HeadSHA = fmt.Sprintf("%040x", i)
		latest = lifecycleRun(t, s, snap, []Finding{f}, "succeeded")
	}
	history, err := s.FindingLifecycle(ctx, latest)
	if err != nil || len(history.Current) != 1 || len(history.Current[0].Occurrences) != 20 || !history.Current[0].OccurrencesTruncated {
		t.Fatal("observation cap missing", history, err)
	}
	snap.MRIID = 2
	snap.HeadSHA = strings.Repeat("b", 40)
	findings := []Finding{}
	for i := 0; i < 55; i++ {
		different := f
		different.ID = fmt.Sprintf("f%d", i)
		different.File = fmt.Sprintf("file%d.any", i)
		findings = append(findings, different)
	}
	lifecycleRun(t, s, snap, findings, "succeeded")
	snap.HeadSHA = strings.Repeat("c", 40)
	absent := lifecycleRun(t, s, snap, []Finding{}, "succeeded")
	history, err = s.FindingLifecycle(ctx, absent)
	if err != nil || len(history.NotReobserved) != 50 || !history.NotReobservedTruncated {
		t.Fatal("prior-only cap missing", len(history.NotReobserved), err)
	}
}
