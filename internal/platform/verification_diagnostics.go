package platform

import (
	"errors"
	"io"
)

// Provenance failures retain the public error text, but record only a finite
// server-selected cause. Never include the rejected ID or provider content.
type verifierObservationError struct{ cause string }

func (e *verifierObservationError) Error() string {
	return "verifier observation is not a fresh successful pinned source"
}

func verificationObservationFailureCode(err error) string {
	var failure *verifierObservationError
	if errors.As(err, &failure) {
		switch failure.cause {
		case "unknown", "wrong_stage", "non_source", "read_failed", "malformed_output", "snapshot_mismatch", "empty_source":
			return "invalid_observation_" + failure.cause
		}
	}
	return "invalid_observation"
}

// Parse failures expose only finite server-selected codes. Existing rejection
// rules and error messages remain authoritative; provider text is never echoed.
func verificationParseFailureCode(err error) string {
	if err == nil {
		return "invalid_structure"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "json_syntax"
	}
	switch err.Error() {
	case "verification response exceeds budget":
		return "response_budget"
	case "trailing verification data":
		return "trailing_data"
	case "invalid verification verdict":
		return "invalid_verdict"
	case "invalid verification claim coverage":
		return "invalid_claim_coverage"
	case "invalid verification explanation":
		return "invalid_explanation"
	case "invalid verification limitation":
		return "invalid_limitation"
	case "too many verification checks":
		return "too_many_checks"
	case "invalid verification check":
		return "invalid_check"
	case "verification check must cite unique linked fresh sources":
		return "invalid_check_sources"
	}
	return jsonFailureCode(err)
}
