package opencode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pausan/agenttik/app/internal/agent"
)

var accountMu sync.Mutex

func (p *Provider) DefaultHome() string {
	if home := os.Getenv("XDG_DATA_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}
func (p *Provider) home(home string) string {
	if home == "" {
		return p.DefaultHome()
	}
	return home
}
func (p *Provider) authPath(home string) string {
	return filepath.Join(p.home(home), "opencode", "auth.json")
}
func (p *Provider) key(home string) string {
	var auth map[string]struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	b, err := os.ReadFile(p.authPath(home))
	if err != nil {
		return ""
	}
	if json.Unmarshal(b, &auth) != nil || auth["opencode-go"].Type != "api" {
		return ""
	}
	return auth["opencode-go"].Key
}
func (p *Provider) AccountStatus(home string) agent.AccountStatus {
	return agent.AccountStatus{SignedIn: p.key(home) != "", Detail: "Go key saved"}
}
func (p *Provider) LoginCommand(home string) agent.LoginCommand {
	login := agent.LoginCommand{Args: []string{Binary, "auth", "login", "--provider", "opencode-go"}}
	if home != "" {
		login.Env = []string{"XDG_DATA_HOME=" + home}
	}
	return login
}
func (p *Provider) ConnectionMode(home string) string {
	b, _ := os.ReadFile(filepath.Join(p.home(home), "opencode", "agenttik-mode"))
	switch string(b) {
	case "direct", "cli":
		return string(b)
	}
	return "auto"
}
func (p *Provider) ConfigureAccount(home, mode, key string) error {
	if mode != "auto" && mode != "direct" && mode != "cli" {
		return fmt.Errorf("connection must be auto, cli, or direct")
	}
	key = strings.TrimSpace(key)
	if strings.ContainsAny(key, "\r\n") || len(key) > 8192 {
		return fmt.Errorf("invalid OpenCode key")
	}
	accountMu.Lock()
	defer accountMu.Unlock()
	if key != "" {
		auth := map[string]json.RawMessage{}
		b, err := os.ReadFile(p.authPath(home))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot read OpenCode login")
		}
		if err == nil && json.Unmarshal(b, &auth) != nil {
			return fmt.Errorf("cannot update invalid OpenCode login file")
		}
		if auth == nil {
			auth = map[string]json.RawMessage{}
		}
		auth["opencode-go"], _ = json.Marshal(map[string]string{"type": "api", "key": key})
		b, _ = json.Marshal(auth)
		if err := privateWrite(p.authPath(home), b); err != nil {
			return err
		}
	}
	return privateWrite(filepath.Join(p.home(home), "opencode", "agenttik-mode"), []byte(mode))
}

func privateWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agenttik-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
