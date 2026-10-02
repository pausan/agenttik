package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

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

// AccountStatus reads which plan a login is on and nothing else. The token
// itself is never decoded, logged or passed on.
//
// Linux and Windows keep the login in .credentials.json. macOS keeps it in
// the Keychain, so there the sign is the account Claude Code records in its
// config file on every platform.
func (p *Provider) AccountStatus(home string) agent.AccountStatus {
	if home == "" {
		home = p.DefaultHome()
	}
	if home == "" {
		return agent.AccountStatus{}
	}
	if body, err := os.ReadFile(filepath.Join(home, ".credentials.json")); err == nil {
		var file struct {
			OAuth *struct {
				SubscriptionType string `json:"subscriptionType"`
			} `json:"claudeAiOauth"`
		}
		if err := json.Unmarshal(body, &file); err != nil || file.OAuth == nil {
			// A file we cannot read is still a login the CLI may well accept,
			// so this reports it as present without a name for it.
			return agent.AccountStatus{SignedIn: true}
		}
		return agent.AccountStatus{SignedIn: true, Detail: planDetail(file.OAuth.SubscriptionType)}
	}
	body, err := os.ReadFile(configFile(home))
	if err != nil {
		return agent.AccountStatus{}
	}
	var config struct {
		Account *struct {
			OrganizationType string `json:"organizationType"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(body, &config) != nil || config.Account == nil {
		return agent.AccountStatus{}
	}
	plan := strings.TrimPrefix(config.Account.OrganizationType, "claude_")
	return agent.AccountStatus{SignedIn: true, Detail: planDetail(plan)}
}

// configFile is Claude Code's .claude.json for a config directory. The
// default directory, ~/.claude, keeps it in the home directory instead.
func configFile(dir string) string {
	if home, err := os.UserHomeDir(); err == nil && dir == filepath.Join(home, ".claude") {
		return filepath.Join(home, ".claude.json")
	}
	return filepath.Join(dir, ".claude.json")
}

func planDetail(plan string) string {
	if plan == "" {
		return ""
	}
	return plan + " plan"
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
