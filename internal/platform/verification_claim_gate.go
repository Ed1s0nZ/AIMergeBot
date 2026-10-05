package platform

// Coverage is an independent model declaration, not proof of semantics. It
// prevents explicit partial support from being presented as full endorsement.
func gateClaimCoverage(v *FindingVerification) {
	if v.ClaimCoverage == "" {
		v.ClaimCoverage = "unknown"
	}
	if v.Status != "supported" || v.ClaimCoverage == "full" {
		return
	}
	note := "Independent verification did not support every assertion in the finding (claim coverage: " + v.ClaimCoverage + ")"
	v.Status = "inconclusive"
	v.Limitations = append(v.Limitations, note)
	reason := []rune(note + ". Model support proposal was not accepted. Unconfirmed model explanation: " + v.Reason)
	if len(reason) > 1000 {
		reason = append(reason[:999], '…')
	}
	v.Reason = string(reason)
}
