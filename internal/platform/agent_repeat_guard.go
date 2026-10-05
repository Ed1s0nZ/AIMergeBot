package platform

import "encoding/json"

const repeatedReadLimit = 3

func guardedRead(name string) bool {
	return isSourceTool(name) || name == "list_files" || name == "list_directory" || name == "list_repositories" || name == "list_repository_directory"
}

// Exact arguments deliberately retain cursor/page and repository/side identity.
func repeatedReadKey(name string, args any) string {
	raw, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return name + ":" + string(raw)
}

func (t *auditTools) repeatedReadBlocked(name string, args any) bool {
	if !guardedRead(name) {
		return false
	}
	key := repeatedReadKey(name, args)
	if key == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.successfulReads[key] >= repeatedReadLimit
}

func (t *auditTools) recordSuccessfulRead(name string, args any) {
	if !guardedRead(name) {
		return
	}
	key := repeatedReadKey(name, args)
	if key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.successfulReads == nil {
		t.successfulReads = map[string]int{}
	}
	t.successfulReads[key]++
}
