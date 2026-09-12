// Package orchestrator holds the built-in instructions and per-turn connection
// details. Preferences never contain a process address or an executable path.
package orchestrator

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
)

//go:embed default.md
var DefaultPrompt string

// Context is refreshed on every turn, including resumed conversations. The
// CLI discovers the live listener from the selected data directory's lock.
func Context(executable, dataDir, sessionID string, projectID int64) string {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	prefix := ""
	if runtime.GOOS == "windows" {
		quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		prefix = "& " // PowerShell's call operator for a quoted executable.
	}
	return fmt.Sprintf(`<agenttik-runtime>
Current task (session) ID: %s
Orchestrator project ID: %d
Use this command prefix for the running app; it replaces any older connection details:
%s%s --data-dir %s
Append --api METHOD, optional --body JSON (or --body - to read JSON from stdin), then the /api/path.
Example: %s%s --data-dir %s --api GET '/api/projects'
Read live state before answering status questions or making changes. Runtime details are supplied by agenttik, separate from the saved project prompt.
</agenttik-runtime>`, sessionID, projectID, prefix, quote(executable), quote(dataDir), prefix, quote(executable), quote(dataDir))
}
