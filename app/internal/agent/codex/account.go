package codex

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Codex keeps its login in auth.json under CODEX_HOME, so that variable is
// the whole of multi-subscription support here. See
// 050-subscription-accounts.md.

// HomeVar is the environment variable the Codex CLI reads its home from.
const HomeVar = "CODEX_HOME"

// DefaultHome is where the CLI keeps its login when nothing overrides it.
func (p *Provider) DefaultHome() string {
	if dir := os.Getenv(HomeVar); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// AccountStatus reads which way the login authenticates — a ChatGPT
// subscription or an API key — and nothing else. The tokens beside it are
// not decoded and never leave the CLI.
func (p *Provider) AccountStatus(home string) agent.AccountStatus {
	if home == "" {
		home = p.DefaultHome()
	}
	if home == "" {
		return agent.AccountStatus{}
	}
	body, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return agent.AccountStatus{}
	}
	var file struct {
		AuthMode string `json:"auth_mode"`
	}
	if err := json.Unmarshal(body, &file); err != nil {
		return agent.AccountStatus{SignedIn: true}
	}
	detail := file.AuthMode
	if detail == "chatgpt" {
		detail = "ChatGPT subscription"
	}
	return agent.AccountStatus{SignedIn: true, Detail: detail}
}

// LoginCommand signs one home in through the CLI's own browser flow.
func (p *Provider) LoginCommand(home string) agent.LoginCommand {
	cmd := agent.LoginCommand{Args: []string{Binary, "login"}}
	if home != "" {
		cmd.Env = []string{HomeVar + "=" + home}
	}
	return cmd
}
