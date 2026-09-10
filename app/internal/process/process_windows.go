//go:build windows

// Package process configures and stops CLI processes across operating systems.
package process

import (
	"os"
	"os/exec"
	"strconv"
)

func Configure(*exec.Cmd) {}

func Terminate(p *os.Process) error {
	return taskkill(p)
}

func Kill(p *os.Process) error {
	return taskkill(p)
}

func taskkill(p *os.Process) error {
	return exec.Command("taskkill.exe", "/pid", strconv.Itoa(p.Pid), "/t", "/f").Run()
}
