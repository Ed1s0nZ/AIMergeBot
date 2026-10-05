package platform

import "encoding/json"

func currentGroupNavigation(group *AuditGroup) string {
	if group == nil {
		return ""
	}
	raw, _ := json.Marshal(group)
	if len(raw) > 16*1024 {
		raw, _ = json.Marshal(map[string]any{"id": group.ID, "files_omitted": len(group.Files)})
	}
	return "\nCurrent audit group (navigation only, not source evidence):\n" + string(raw) + "\nOnly changed lines in this group's diff are eligible for submission. The whole-PR manifest is navigation, not the current submission scope. Prior accepted findings are retained by the server: do not resubmit them in this group. You may read other files to establish impact or counterevidence, but source availability does not grant a new changed-line anchor. Re-read sources with new observation IDs for any new investigation."
}

func uniqueCoverageNotes(notes []string) []string {
	out := make([]string, 0, len(notes))
	seen := make(map[string]bool, len(notes))
	for _, note := range notes {
		if !seen[note] {
			out = append(out, note)
			seen[note] = true
		}
	}
	return out
}
