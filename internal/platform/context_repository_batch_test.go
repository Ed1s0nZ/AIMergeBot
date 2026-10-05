package platform

import (
	"context"
	"strings"
	"testing"
)

func TestContextBatchPinnedEvidenceAndSingleObservation(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	out, err := tools.contextBatch(context.Background(), contextBatchArgs{RepositoryID: 2, Files: []readArgs{{Path: "guard.any", Start: 1, End: 3}, {Path: "guard.any", Start: 2, End: 2}}})
	if err != nil || out.Error != "" || !out.EvidenceEligible || out.RepositoryID != 2 || out.HeadSHA != sources[2].Snapshot.HeadSHA || !strings.Contains(out.Text, "context-only needle") || strings.Contains(out.Text, "latest safe") {
		t.Fatalf("unexpected batch: %+v, %v", out, err)
	}
	if tools.calls != 1 || len(tools.trace) != 1 {
		t.Fatalf("batch consumed nested observations: calls=%d traces=%d", tools.calls, len(tools.trace))
	}
}

func TestContextBatchRejectsInvalidRequestsWithoutSource(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    int
		files []readArgs
	}{
		{"unknown", 99, []readArgs{{Path: "guard.any", Start: 1, End: 2}}},
		{"empty", 2, nil},
		{"too_many", 2, make([]readArgs, 9)},
		{"base", 2, []readArgs{{Path: "guard.any", Base: true, Start: 1, End: 2}}},
		{"missing_after_valid", 2, []readArgs{{Path: "guard.any", Start: 1, End: 2}, {Path: "missing.any", Start: 1, End: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, snap, _, sources := crossGitFixture(t)
			out, _ := crossTools(root, snap, sources).contextBatch(context.Background(), contextBatchArgs{RepositoryID: tc.id, Files: tc.files})
			if out.Error == "" || out.Text != "" || out.EvidenceEligible {
				t.Fatalf("invalid batch leaked evidence: %+v", out)
			}
		})
	}
}

func TestContextBatchRechecksAuthorizationAfterCachedReads(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	source := sources[2]
	calls := 0
	source.Authorize = func(context.Context) error {
		calls++
		if calls >= 2 {
			return ErrContextRepository
		}
		return nil
	}
	sources[2] = source
	out, _ := crossTools(root, snap, sources).contextBatch(context.Background(), contextBatchArgs{RepositoryID: 2, Files: []readArgs{{Path: "guard.any", Start: 1, End: 3}}})
	if calls != 2 || out.Error == "" || out.Text != "" || out.EvidenceEligible {
		t.Fatalf("revoked batch: calls=%d out=%+v", calls, out)
	}
}

func TestContextBatchPreservesPartialCoverageAndRejectsOversize(t *testing.T) {
	root, snap, _, sources := crossGitFixture(t)
	tools := crossTools(root, snap, sources)
	reader, _, err := tools.contextReader(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	reader.cache["false:guard.any"] = strings.Repeat("line\n", 300)
	out, _ := tools.contextBatch(context.Background(), contextBatchArgs{RepositoryID: 2, Files: []readArgs{{Path: "guard.any", Start: 1, End: 250}, {Path: "guard.any", Start: 299, End: 300}}})
	if out.Error != "" || !out.More || !out.Remaining || !strings.Contains(out.Text, "300: line") {
		t.Fatalf("lost partial state or skipped later file: %+v", out)
	}
	reader.cache["false:guard.any"] = strings.Repeat("x", 9000) + "\n"
	out, _ = tools.contextBatch(context.Background(), contextBatchArgs{RepositoryID: 2, Files: []readArgs{{Path: "guard.any", Start: 1, End: 1}, {Path: "guard.any", Start: 1, End: 1}}})
	if out.Error == "" || out.Text != "" || out.EvidenceEligible {
		t.Fatalf("oversize batch leaked partial evidence: %+v", out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, _ = tools.contextBatch(ctx, contextBatchArgs{RepositoryID: 2, Files: []readArgs{{Path: "guard.any", Start: 1, End: 1}}})
	if out.Error == "" || out.Text != "" {
		t.Fatalf("cancelled cached read returned source: %+v", out)
	}
}
