//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

// ConfigureProcess puts the child in its own process group so descendants can
// be killed with it.
func ConfigureProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// AfterProcessStart is a no-op on Unix; process groups need no assignment.
func AfterProcessStart(cmd *exec.Cmd) error { return nil }

// TerminateProcessTree kills the child's process group, reaping descendants
// that inherited its stdout/stderr.
func TerminateProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
