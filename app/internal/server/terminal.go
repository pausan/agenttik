package server

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Signing a subscription in needs a terminal. Every CLI here draws its login
// on one and answers keypresses, and none of them prints its URL when stdout
// is a pipe, so relaying the flow through the web UI would mean shipping a
// terminal emulator. Handing the command to the desktop's own terminal is
// both smaller and the thing the user would have done anyway.
//
// This is the sibling of openInSystem in api_open.go: the same "ask the
// desktop" idea, for a command instead of a path. A machine with no terminal
// to open is not an error worth failing the request over — the UI shows the
// command either way. See 050-subscription-accounts.md.

// openTerminal starts cmd in a terminal window on the machine the CLIs are
// installed on.
func openTerminal(cmd agent.LoginCommand) error {
	if len(cmd.Args) == 0 {
		return errors.New("no login command")
	}
	// One shell line, so the environment, the CLI and the hold that keeps the
	// window open after it exits are one argument to whatever opens it.
	line := shellLine(cmd) + loginHold
	for _, candidate := range terminalCandidates(line) {
		if _, err := exec.LookPath(candidate[0]); err != nil {
			continue
		}
		started := exec.Command(candidate[0], candidate[1:]...)
		if err := started.Start(); err != nil {
			continue
		}
		// Nothing waits for a terminal the user is typing into; reaping it
		// keeps a zombie from sitting there for the life of the app, as
		// openInSystem does for the desktop's file handler.
		go started.Wait()
		return nil
	}
	return fmt.Errorf("no terminal application found to run: %s", shellLine(cmd))
}

// terminalCandidates is the list to try, in order, each already carrying the
// shell line. The first that exists on PATH and starts wins.
func terminalCandidates(line string) [][]string {
	switch runtime.GOOS {
	case "darwin":
		// Terminal.app takes a script rather than argv, and raising it is
		// part of opening one.
		script := fmt.Sprintf(`tell application "Terminal"
activate
do script %s
end tell`, appleScriptString(line))
		return [][]string{{"osascript", "-e", script}}
	case "windows":
		// /k keeps the window after the command, which is what loginHold does
		// for the others, so the line is passed without it.
		return [][]string{
			{"cmd", "/c", "start", "", "cmd", "/k", line},
		}
	default:
		// x-terminal-emulator first: on Debian and Ubuntu it is whichever
		// terminal the user actually chose. The rest cover the common
		// desktops, ending with xterm, which is what exists when nothing
		// else does.
		return [][]string{
			{"x-terminal-emulator", "-e", "sh", "-c", line},
			{"gnome-terminal", "--", "sh", "-c", line},
			{"konsole", "-e", "sh", "-c", line},
			{"xfce4-terminal", "-e", "sh -c " + shellQuote(line)},
			{"kitty", "sh", "-c", line},
			{"alacritty", "-e", "sh", "-c", line},
			{"wezterm", "start", "--", "sh", "-c", line},
			{"ghostty", "-e", "sh", "-c", line},
			{"xterm", "-e", "sh", "-c", line},
		}
	}
}

// appleScriptString quotes a shell line for `do script`. AppleScript strings
// escape with a backslash, and the line already contains single quotes of its
// own, so only the double quote and the backslash need care.
func appleScriptString(line string) string {
	quoted := make([]rune, 0, len(line)+2)
	quoted = append(quoted, '"')
	for _, r := range line {
		if r == '"' || r == '\\' {
			quoted = append(quoted, '\\')
		}
		quoted = append(quoted, r)
	}
	return string(append(quoted, '"'))
}
