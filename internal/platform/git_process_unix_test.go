//go:build darwin || linux

package platform

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestGitProcessWaitDelayBoundsInheritedPipes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 &")
	configureGitProcess(cmd)
	if cmd.WaitDelay != 2*time.Second || cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("missing Git process lifecycle policy")
	}
	// Shorten only this test's drain window; the child keeps the pipe open.
	cmd.WaitDelay = 100 * time.Millisecond
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	defer func() {
		if cmd.Process != nil && cmd.Process.Pid > 1 {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
	if err := cmd.Run(); !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("inherited pipe should report ErrWaitDelay, got %v", err)
	}
}

func TestGitProcessCancelWithoutProcessIsSafe(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "git", "--version")
	configureGitProcess(cmd)
	if !errors.Is(cmd.Cancel(), os.ErrProcessDone) {
		t.Fatal("cancel without a process must not signal a process group")
	}
}
