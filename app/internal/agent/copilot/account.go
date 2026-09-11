package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Copilot is the one provider here whose token does not live in its config
// directory: the CLI puts it in the machine's own vault (keychain, keyring,
// credential manager), keyed by the account it belongs to, and records in
// `<dir>/config.json` which account that is. So the directory still selects
// the subscription — two of them name two logins the vault already holds —
// and `--config-dir` is how a turn picks one. `COPILOT_HOME` does the same
// job; the flag is used because it is visible in the command.
//
// The consequence worth knowing: signing a second account in does not
// displace the first, but both entries live in one vault, so a vault the
// CLI cannot reach falls back to a plaintext file under the config directory
// — the CLI's own behaviour, not something changed here. See
// 050-subscription-accounts.md.

// HomeVar is the environment variable the Copilot CLI reads its config
// directory from, equivalent to --config-dir.
const HomeVar = "COPILOT_HOME"

// DefaultHome is where the CLI keeps its config when nothing overrides it.
func (p *Provider) DefaultHome() string {
	if dir := os.Getenv(HomeVar); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".copilot")
}

// AccountStatus names the GitHub login the CLI last signed in with in this
// directory, which is the identity it will use again. No token is read: the
// one this refers to is in the machine's vault, which only the CLI opens.
func (p *Provider) AccountStatus(home string) agent.AccountStatus {
	if home == "" {
		home = p.DefaultHome()
	}
	if home == "" {
		return agent.AccountStatus{}
	}
	body, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		return agent.AccountStatus{}
	}
	var file struct {
		LastLoggedInUser *struct {
			Login string `json:"login"`
			Host  string `json:"host"`
		} `json:"lastLoggedInUser"`
	}
	if err := json.Unmarshal(body, &file); err != nil || file.LastLoggedInUser == nil ||
		file.LastLoggedInUser.Login == "" {
		return agent.AccountStatus{}
	}
	detail := file.LastLoggedInUser.Login
	// The host is only worth naming when it is not github.com — an
	// Enterprise server. The CLI writes it as a URL, so the scheme comes off
	// before that comparison or every ordinary login reads as a custom one.
	if host := trimHost(file.LastLoggedInUser.Host); host != "" && host != "github.com" {
		detail += " @ " + host
	}
	return agent.AccountStatus{SignedIn: true, Detail: detail}
}

// trimHost reduces "https://github.com/" to "github.com".
func trimHost(host string) string {
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	return strings.TrimSuffix(host, "/")
}

// LoginCommand signs one config directory in through the CLI's device flow.
func (p *Provider) LoginCommand(home string) agent.LoginCommand {
	cmd := agent.LoginCommand{Args: []string{Binary, "login"}}
	if home != "" {
		cmd.Args = append(cmd.Args, "--config-dir", home)
	}
	return cmd
}

// homeArgs is --config-dir for an account that has one, and nothing for the
// machine's own login, so a single-subscription machine runs the command it
// always ran.
func homeArgs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{"--config-dir", home}
}
