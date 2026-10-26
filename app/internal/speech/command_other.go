//go:build !windows

package speech

import "os/exec"

func configureCommand(*exec.Cmd) {}
