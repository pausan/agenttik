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
// flatID is the orchestrator's project in the flat view, or 0 when this
// machine has nothing to flatten. See specs/100-flat-view.md.
func Context(executable, dataDir, sessionID string, projectID, flatID int64) string {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	prefix := ""
	if runtime.GOOS == "windows" {
		quote = func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		prefix = "& " // PowerShell's call operator for a quoted executable.
	}
	if flatID == 0 {
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
	return fmt.Sprintf(`<agenttik-runtime>
Current task (session) ID: %s
Orchestrator project ID: %d
Use this command prefix for the running app; it replaces any older connection details:
%s%s --data-dir %s
Append --api METHOD, optional --body JSON (or --body - to read JSON from stdin), then the /api/path.
Flat view: you act on every profile of this machine and of its connected remote machines at once. Add profile=all to the query of every /api/ path, for example '/api/projects?profile=all' or '/api/sessions/TASK_ID?profile=all'. IDs read from the flat view are valid only with profile=all.
Example: %s%s --data-dir %s --api GET '/api/projects?profile=all'
Each project in the flat view has a source naming its machine and profile. Projects with the same name or folder in different places are different projects: choose the one the request is about, and ask when it is unclear. Tasks and jobs run on their project's machine. New projects are added to this machine's Default profile.
Read live state before answering status questions or making changes. Runtime details are supplied by agenttik, separate from the saved project prompt.
</agenttik-runtime>`, sessionID, flatID, prefix, quote(executable), quote(dataDir), prefix, quote(executable), quote(dataDir))
}
