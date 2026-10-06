package platform

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestVerificationParseDiagnosticsPreserveGateAndPrivacy(t *testing.T) {
	const private = "PRIVATE_PROVIDER_VALUE"
	good := verificationInput{Status: "inconclusive", ClaimCoverage: "partial", Reason: private, Limitations: []string{}, ObservationIDs: []string{private}, Checks: supportedChecks(private)}
	encode := func(v verificationInput) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	rawGood := encode(good)
	if _, err := parseVerification(rawGood); err != nil {
		t.Fatal("valid response changed", err)
	}
	cases := []struct{ code, raw string }{
		{"json_syntax", `{"reason":"` + private + `",`},
		{"json_type", `{"status":123,"reason":"` + private + `"}`},
		{"unknown_field", strings.TrimSuffix(rawGood, "}") + `,"` + private + `":true}`},
		{"response_budget", strings.Repeat(private, 1000)},
		{"trailing_data", rawGood + `{}`},
	}
	for _, code := range []string{"invalid_verdict", "invalid_claim_coverage", "invalid_explanation", "invalid_limitation", "too_many_checks", "invalid_check", "invalid_check_sources"} {
		v := good
		v.Checks = cloneVerificationChecks(good.Checks)
		switch code {
		case "invalid_verdict":
			v.Status = private
		case "invalid_claim_coverage":
			v.ClaimCoverage = private
		case "invalid_explanation":
			v.Reason = strings.Repeat(private, 60)
		case "invalid_limitation":
			v.Limitations = []string{strings.Repeat(private, 20)}
		case "too_many_checks":
			v.Checks = append(v.Checks, v.Checks[0])
		case "invalid_check":
			v.Checks[0].Reason = strings.Repeat(private, 30)
		case "invalid_check_sources":
			v.Checks[0].ObservationIDs = []string{private + "_unlinked"}
		}
		cases = append(cases, struct{ code, raw string }{code, encode(v)})
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			_, err := parseVerification(tc.raw)
			if err == nil {
				t.Fatal("malformed response accepted")
			}
			code := verificationParseFailureCode(err)
			if code != tc.code {
				t.Fatalf("classification %s want %s", code, tc.code)
			}
			diagnostic := responseDiagnostic(tc.raw, responseError(code))
			if strings.Contains(diagnostic, private) || len(diagnostic) > 300 {
				t.Fatal("private or unbounded diagnostic", diagnostic)
			}
			var shape map[string]any
			if json.Unmarshal([]byte(diagnostic), &shape) != nil || shape["code"] != code || shape["bytes"] != float64(len(tc.raw)) {
				t.Fatal("invalid diagnostic shape")
			}
		})
	}
	for _, err := range []error{nil, errors.New(private)} {
		code := verificationParseFailureCode(err)
		if strings.Contains(code, private) || (code != "json_decode" && code != "invalid_structure") {
			t.Fatal("unsafe fallback", code)
		}
	}
}
