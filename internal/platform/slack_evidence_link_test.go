package platform

import "testing"

func TestSlackEvidenceLinkUsesOnlyValidPlatformBase(t *testing.T) {
	for _, base := range []string{"", "https://user:secret@example.com", "https://example.com/?token=secret", "https://example.com/#fragment", "javascript:alert(1)", "https://example.com/\n<@everyone>", "https://example.com/<tag>", "https://example.com/|label"} {
		if out := slackEvidenceLink(base, 12); out != "" {
			t.Fatal("unsafe link", out)
		}
	}
	if out := slackEvidenceLink("https://audit.example.com/platform/", 12); out != "https://audit.example.com/platform/#/runs/12" {
		t.Fatal(out)
	}
	if out := slackEvidenceLink("https://audit.example.com", 0); out != "" {
		t.Fatal(out)
	}
}
