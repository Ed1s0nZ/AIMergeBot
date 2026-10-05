package platform

import (
	"encoding/json"
	"errors"
	"strings"
)

// AuditResponseError never contains provider text, field names or snippets.
type AuditResponseError struct{ Code string }

func (e *AuditResponseError) Error() string { return "invalid audit response: " + e.Code }
func responseError(code string) error       { return &AuditResponseError{Code: code} }
func jsonFailureCode(err error) string {
	var syntax *json.SyntaxError
	var kind *json.UnmarshalTypeError
	if errors.As(err, &syntax) {
		return "json_syntax"
	}
	if errors.As(err, &kind) {
		return "json_type"
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return "unknown_field"
	}
	return "json_decode"
}
func responseDiagnostic(raw string, err error) string {
	code := "invalid_structure"
	var typed *AuditResponseError
	if errors.As(err, &typed) {
		code = typed.Code
	}
	shape := struct {
		Code      string `json:"code"`
		Bytes     int    `json:"bytes"`
		Fenced    bool   `json:"fenced"`
		ValidJSON bool   `json:"valid_json"`
	}{code, len(raw), strings.HasPrefix(strings.TrimSpace(raw), "```"), false}
	if len(raw) <= 128*1024 {
		shape.ValidJSON = json.Valid([]byte(raw))
	}
	b, _ := json.Marshal(shape)
	return string(b)
}
