package platform

import "regexp"

var primaryRecordingObservationID = regexp.MustCompile(`^(group-[1-9][0-9]*-)?observation-[1-9][0-9]*$`)

// Caller holds t.mu. IDs alone cross the trusted navigation boundary. Arbitrary
// configured prefixes are not projected, even when valid for tool invocation.
func (t *auditTools) eligibleRecordingCorrectionsLocked() []recordingCorrection {
	var pairs []recordingCorrection
	for i, failed := range t.trace {
		if failed.Error == "" || recordingFamily(failed.Name) == "" || len(failed.ObservationID) > 80 || !primaryRecordingObservationID.MatchString(failed.ObservationID) {
			continue
		}
		for j := len(t.trace) - 1; j > i; j-- {
			corrected := t.trace[j]
			if len(corrected.ObservationID) > 80 || !primaryRecordingObservationID.MatchString(corrected.ObservationID) {
				continue
			}
			pair := recordingCorrection{failed.ObservationID, corrected.ObservationID}
			if _, err := t.recordingCorrectionKeyLocked(pair); err != nil {
				continue
			}
			pairs = append(pairs, pair)
			break
		}
		if len(pairs) == 4 {
			break
		}
	}
	return pairs
}
