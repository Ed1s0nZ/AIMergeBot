package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResumeCannotMixGroupedAndSingleAuditEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	meta := metadata{Grouped: true, CorpusDigest: "same", CodeRevision: "same", MaxToolCalls: 80}
	if err := save(path, meta); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	if err := checkResumeMetadata(path, meta); err != nil {
		t.Fatal(err)
	}
	meta.Grouped = false
	if err := checkResumeMetadata(path, meta); err == nil {
		t.Fatal("mixed grouping evidence accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("failed resume rewrote prior evidence")
	}
	if err := os.WriteFile(path, []byte(`{"grouped":true`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkResumeMetadata(path, meta); err == nil {
		t.Fatal("corrupt metadata accepted")
	}
}
