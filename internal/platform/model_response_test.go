package platform

import (
	"strings"
	"testing"
)

func TestModelResponseDiagnosticsNeverEchoContent(t *testing.T) {
	for _, raw := range []string{`{"sk-secret-reflected":1}`, "```json\n{sk-secret-reflected}\n```", `{"findings":"sk-secret-reflected","summary":"x","coverage_notes":[]}`} {
		_, err := ParseResult(raw)
		if err == nil {
			t.Fatal("malformed output accepted")
		}
		if strings.Contains(err.Error(), "sk-secret-reflected") || strings.Contains(responseDiagnostic(raw, err), "sk-secret-reflected") {
			t.Fatal("response content leaked")
		}
	}
}
