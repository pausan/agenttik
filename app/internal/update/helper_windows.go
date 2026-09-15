//go:build windows

package update

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func launchHelper(ctx context.Context, helper, manifest, target string) error {
	if writableDirectory(target) {
		cmd := exec.Command(helper, "--internal-apply-update", manifest)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS, HideWindow: true}
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	}
	// EncodedCommand avoids PowerShell/native command-line quoting differences.
	args := "--internal-apply-update " + syscall.EscapeArg(manifest)
	script := "$ErrorActionPreference='Stop'; Start-Process -FilePath " + psQuote(helper) + " -ArgumentList " + psQuote(args) + " -Verb RunAs"
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(data[i*2:], u)
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("administrator approval failed: %s: %w", out, err)
	}
	return nil
}
func waitParent(pid int) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err == windows.ERROR_INVALID_PARAMETER {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	_, err = windows.WaitForSingleObject(h, windows.INFINITE)
	return err
}
