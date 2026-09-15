//go:build !windows

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func launchHelper(ctx context.Context, helper, manifest, target string) error {
	cmd := exec.Command(helper, "--internal-apply-update", manifest)
	if !writableDirectory(target) {
		if runtime.GOOS == "darwin" {
			script := shellQuote(helper) + " --internal-apply-update " + shellQuote(manifest) + " >/dev/null 2>&1 &"
			cmd = exec.CommandContext(ctx, "/usr/bin/osascript", "-e", "do shell script "+strconv.Quote(script)+" with administrator privileges")
		} else {
			pkexec, err := exec.LookPath("pkexec")
			if err != nil {
				return errors.New("this installation needs administrator permissions; install polkit (pkexec) or move Agenttik to a user-owned directory")
			}
			cmd = exec.Command(pkexec, helper, "--internal-apply-update", manifest)
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start updater: %w", err)
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			_ = os.WriteFile(manifest+".error", []byte("Updater could not start or administrator approval was cancelled: "+err.Error()), 0600)
		}
	}()
	return nil
}
func waitParent(pid int) error {
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
}
