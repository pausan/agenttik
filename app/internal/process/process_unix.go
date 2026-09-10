//go:build !windows

// Package process configures and stops CLI processes across operating systems.
package process

import (
	"os"
	"os/exec"
	"syscall"
)

func Configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func Terminate(p *os.Process) error {
	return syscall.Kill(-p.Pid, syscall.SIGTERM)
}

func Kill(p *os.Process) error {
	return syscall.Kill(-p.Pid, syscall.SIGKILL)
}
