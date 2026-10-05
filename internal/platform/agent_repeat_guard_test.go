package platform

import (
	"fmt"
	"strings"
	"testing"
)

func TestRepeatedSnapshotReadsStopWithoutRepeatingIO(t *testing.T) {
	if !guardedRead("list_repository_directory") {
		t.Fatal("context directory unguarded")
	}
	tools := &auditTools{snap: Snapshot{BaseSHA: "b", HeadSHA: "h"}}
	calls := 0
	args := readArgs{Path: "a.any", Start: 1, End: 2}
	for i := 0; i < 5; i++ {
		out, err := tools.invoke("read_file", args, func() (toolOutput, error) { calls++; return toolOutput{Text: "source"}, nil })
		if err != nil {
			t.Fatal(err)
		}
		if i >= 3 && (out.Error == "" || out.Text != "" || out.EvidenceEligible || len(out.EligibleObservationIDs) != 3) {
			t.Fatalf("not blocked safely: %+v", out)
		}
	}
	if calls != 3 || len(tools.trace) != 5 || len(tools.unresolved()) != 1 {
		t.Fatal("repeat not recorded", calls)
	}
	out, _ := tools.invoke("read_file", readArgs{Path: "a.any", Base: true, Start: 1, End: 2}, func() (toolOutput, error) { calls++; return toolOutput{Text: "base"}, nil })
	if out.Error != "" || calls != 4 {
		t.Fatal("base incorrectly combined")
	}
}

func TestRepeatedReadsKeepPaginationAndRetriesDistinct(t *testing.T) {
	tools := &auditTools{}
	for i := 0; i < 5; i++ {
		tools.invoke("search_code", searchArgs{Query: "q", Page: i}, func() (toolOutput, error) { return toolOutput{Text: "match"}, nil })
	}
	for i := 0; i < 4; i++ {
		tools.invoke("read_file", readArgs{Path: "retry.any"}, func() (toolOutput, error) { return toolOutput{}, fmt.Errorf("temporary") })
	}
	out, _ := tools.invoke("read_file", readArgs{Path: "retry.any"}, func() (toolOutput, error) { return toolOutput{Text: "recovered"}, nil })
	if out.Error != "" || out.Text != "recovered" {
		t.Fatal("failures blocked recovery")
	}
	for i := 0; i < 5; i++ {
		out, _ := tools.invoke("submit_finding", struct{}{}, func() (toolOutput, error) { return toolOutput{Text: "updated"}, nil })
		if out.Error != "" {
			t.Fatal("write guarded")
		}
	}
	if repeatedReadKey("search_code", searchArgs{Cursor: 1}) == repeatedReadKey("search_code", searchArgs{Cursor: 2}) {
		t.Fatal("cursor lost")
	}
	if !strings.Contains(tools.trace[0].Output, "match") {
		t.Fatal("source lost")
	}
}
