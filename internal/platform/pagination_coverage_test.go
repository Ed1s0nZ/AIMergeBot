package platform

import "testing"

func TestContextPaginationCannotUseAnotherRepositoryToFillGaps(t *testing.T) {
	for _, name := range []string{"search_repository_code", "list_repository_directory"} {
		t.Run(name, func(t *testing.T) {
			tools := &auditTools{}
			first := contextSearchArgs{RepositoryID: 2, Query: "q"}
			key := queryKey(name, first)
			ok, tracked := tools.paginationComplete(name, key, first, toolOutput{Text: "aaaa", More: true, NextCursor: 4}, false)
			if ok || !tracked {
				t.Fatal("first context page marked complete")
			}
			last := first
			last.Cursor = 8
			if queryKey(name, last) != key {
				t.Fatal("cursor split one query")
			}
			tools.paginationComplete(name, key, last, toolOutput{Text: "zzzz"}, false)
			other := first
			other.RepositoryID = 3
			other.Cursor = 4
			otherKey := queryKey(name, other)
			if otherKey == key {
				t.Fatal("repository omitted from query identity")
			}
			tools.paginationComplete(name, otherKey, other, toolOutput{Text: "bbbb", More: true, NextCursor: 8}, false)
			ok, _ = tools.paginationComplete(name, key, last, toolOutput{Text: "zzzz"}, false)
			if ok {
				t.Fatal("other repository filled missing context page")
			}
			middle := first
			middle.Cursor = 4
			ok, _ = tools.paginationComplete(name, key, middle, toolOutput{Text: "bbbb", More: true, NextCursor: 8}, true)
			if ok {
				t.Fatal("failed page filled gap")
			}
			ok, _ = tools.paginationComplete(name, key, middle, toolOutput{Text: "bbbb", More: true, NextCursor: 8}, false)
			if !ok {
				t.Fatal("continuous context query remained incomplete")
			}
		})
	}
}

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
