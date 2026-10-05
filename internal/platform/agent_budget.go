package platform

// Eino counts model and tool nodes separately. A configured round is one model
// decision, optionally followed by tools; the last round must return its report.
func agentGraphSteps(rounds int) int {
	if rounds < 2 {
		rounds = 2
	}
	if rounds > 100 {
		rounds = 100
	}
	return 2*rounds - 1
}
