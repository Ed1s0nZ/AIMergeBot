package platform

import "testing"

func TestSkippedPagesCannotClearCoverageGap(t *testing.T) {
	tools := &auditTools{}
	key := "search"
	first := searchArgs{Query: "q"}
	ok, _ := tools.paginationComplete("search_code", key, first, toolOutput{Text: "aaaa", More: true, NextCursor: 4}, false)
	if ok {
		t.Fatal("first page complete")
	}
	last := first
	last.Cursor = 8
	ok, _ = tools.paginationComplete("search_code", key, last, toolOutput{Text: "zzzz"}, false)
	if ok {
		t.Fatal("skipped middle page cleared coverage")
	}
	middle := first
	middle.Cursor = 4
	ok, _ = tools.paginationComplete("search_code", key, middle, toolOutput{Text: "bbbb", More: true, NextCursor: 8}, false)
	if !ok {
		t.Fatal("continuous recovered coverage still incomplete")
	}
}
func TestFileListMustStartAtFirstPage(t *testing.T) {
	tools := &auditTools{}
	ok, _ := tools.paginationComplete("list_files", "list", listArgs{Page: 3}, toolOutput{}, false)
	if ok {
		t.Fatal("last page alone complete")
	}
	tools.paginationComplete("list_files", "list", listArgs{Page: 1}, toolOutput{More: true}, false)
	ok, _ = tools.paginationComplete("list_files", "list", listArgs{Page: 2}, toolOutput{More: true}, false)
	if !ok {
		t.Fatal("all pages still incomplete")
	}
}
