//go:build darwin || linux

package platform

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureGitProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil || cmd.Process.Pid <= 1 {
			return os.ErrProcessDone
		}
		// Git's HTTP/index-pack helpers normally inherit this dedicated group.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
