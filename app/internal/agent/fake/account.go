package fake

import (
	"os"
	"path/filepath"

	"github.com/pausan/agenttik/app/internal/agent"
)

// The fake provider holds subscriptions the way the real ones do — an alias
// and a directory — so the end-to-end suite can add one, swap to it and watch
// a turn run on it without a CLI on PATH or an allowance to spend. See
// 050-subscription-accounts.md.

// DefaultHome is where this provider pretends its own login lives. Under the
// temporary directory, since nothing is ever written to it.
func (p *Provider) DefaultHome() string {
	return filepath.Join(os.TempDir(), "agenttik-fake-account")
}

// AccountStatus reports every directory as signed in, named after itself, so
// a browser test sees a stable badge and detail per account rather than
// whatever the machine happens to have configured.
func (p *Provider) AccountStatus(home string) agent.AccountStatus {
	if home == "" {
		return agent.AccountStatus{SignedIn: true, Detail: "fake system login"}
	}
	return agent.AccountStatus{SignedIn: true, Detail: filepath.Base(home) + " login"}
}

// LoginCommand names a command that does nothing and exits, so a test that
// reaches the sign-in path cannot start a real login flow.
func (p *Provider) LoginCommand(home string) agent.LoginCommand {
	return agent.LoginCommand{Args: []string{"true"}, Env: []string{"AGENTTIK_FAKE_HOME=" + home}}
}
