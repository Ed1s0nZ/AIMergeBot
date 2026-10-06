package platform

import "errors"

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
