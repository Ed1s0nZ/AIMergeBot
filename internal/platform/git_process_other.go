//go:build !darwin && !linux

package platform

import "os/exec"

// Other platforms keep CommandContext's direct-process cancellation.
// WaitDelay still bounds pipe draining; it does not terminate a process tree.
func configureGitProcessGroup(cmd *exec.Cmd) {}
