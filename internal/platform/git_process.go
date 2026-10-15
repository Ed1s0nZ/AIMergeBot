package platform

import (
	"os/exec"
	"time"
)

// Bound pipe draining even if an exited Git process leaves a helper holding it.
func configureGitProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
	configureGitProcessGroup(cmd)
}
