package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Claude Code keeps everything about one login — the credential, the
// settings, the history — under a single directory, and CLAUDE_CONFIG_DIR
// says which. That one variable is therefore the whole of multi-subscription
// support here: a turn given an account home runs the same command with the
// variable set. See 050-subscription-accounts.md.

// HomeVar is the environment variable Claude Code reads its config directory
// from.
const HomeVar = "CLAUDE_CONFIG_DIR"

// DefaultHome is where the CLI keeps its login when nothing overrides it. An
// override already in this process's environment wins, because that is what
// the CLI would do with it too.
func (p *Provider) DefaultHome() string {
	if dir := os.Getenv(HomeVar); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// AccountStatus reads the two non-secret fields Claude Code records beside
// the token — which plan the login is on — and nothing else. The tokens in
// the same file are not decoded, not logged and never leave the CLI.
func (p *Provider) AccountStatus(home string) agent.AccountStatus {
	if home == "" {
		home = p.DefaultHome()
	}
	if home == "" {
		return agent.AccountStatus{}
	}
	body, err := os.ReadFile(filepath.Join(home, ".credentials.json"))
	if err != nil {
		return agent.AccountStatus{}
	}
	var file struct {
		OAuth *struct {
			SubscriptionType string `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(body, &file); err != nil || file.OAuth == nil {
		// A file we cannot read is still a login the CLI may well accept, so
		// this reports it as present without a name for it.
		return agent.AccountStatus{SignedIn: true}
	}
	detail := file.OAuth.SubscriptionType
	if detail != "" {
		detail += " plan"
	}
	return agent.AccountStatus{SignedIn: true, Detail: detail}
}

// LoginCommand signs one directory in. `/login` is the CLI's own command for
// it; a directory that has never been used runs the same flow on its first
// bare start.
func (p *Provider) LoginCommand(home string) agent.LoginCommand {
	cmd := agent.LoginCommand{Args: []string{Binary, "/login"}}
	if home != "" {
		cmd.Env = []string{HomeVar + "=" + home}
	}
	return cmd
}
