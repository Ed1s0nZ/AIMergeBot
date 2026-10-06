package platform

import (
	"errors"
	"os/exec"
)

var ErrRepositoryUnavailable = errors.New("fixed repository execution unavailable")

// EX_UNAVAILABLE and a missing executable indicate the reader itself cannot run.
// A missing requested file (typically exit128) is an ordinary repairable tool error.
func unavailableGitExecution(err error) bool {
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 69
}
