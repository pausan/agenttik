package agent

import "os"

// One machine can be signed in to two subscriptions of the same provider — a
// company account and a personal one — because every CLI here keeps its login
// in a directory that can be pointed elsewhere: CLAUDE_CONFIG_DIR,
// CODEX_HOME, `copilot --config-dir`. A turn names the directory it wants and
// the provider applies it the way its own CLI expects.
//
// The credential boundary does not move. agenttik hands over a path, and the
// CLI reads and signs with whatever is inside it, exactly as it does for the
// account it was already using. See 050-subscription-accounts.md.

// AccountStatus is what can be said about the login in one directory without
// reading a secret out of it. Detail is a short, non-secret identity the CLI
// happens to record beside the token — a plan name, a login, an auth mode —
// and is empty when it records none.
type AccountStatus struct {
	SignedIn bool   `json:"signed_in"`
	Detail   string `json:"detail,omitempty"`
}

// LoginCommand is what a human runs to sign one account in. It is handed over
// rather than driven: all three CLIs render their login on a terminal and
// answer a keypress, so a browser cannot stand in for one. Env is the
// variables to set, in KEY=VALUE form; Args is the command and its arguments.
type LoginCommand struct {
	Env  []string `json:"env,omitempty"`
	Args []string `json:"args"`
}

// MultiAccount is the optional half of Provider for backends whose login
// lives in a directory, which is what lets one machine hold more than one
// subscription for them. A provider that does not implement it offers only
// the account its CLI is already signed in to.
type MultiAccount interface {
	// DefaultHome is where the CLI keeps its login when nothing overrides it.
	// Shown beside the system account, so a second subscription can be put
	// somewhere else on purpose rather than by guessing.
	DefaultHome() string
	// AccountStatus reports whether the login in home is present, and names
	// it if the CLI records a non-secret identity. An empty home is the
	// CLI's own default directory. It never reads a token.
	AccountStatus(home string) AccountStatus
	// LoginCommand is the command that signs home in.
	LoginCommand(home string) LoginCommand
}

// HomeEnv is the environment for a CLI told to use the login kept in home:
// this process's environment with one variable overridden. An empty home is
// the machine's own login and returns nil, which is exec's own way of saying
// "inherit mine unchanged" — so a single-subscription machine runs exactly
// the command it always did.
func HomeEnv(key, home string) []string {
	if home == "" {
		return nil
	}
	return append(os.Environ(), key+"="+home)
}
