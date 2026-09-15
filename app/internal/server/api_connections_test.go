package server

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/opencode"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestDirectConnectionAPI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reg := agent.NewRegistry(opencode.New(), twoLoginProvider{singleLoginProvider{name: "claude"}})
	s := New(st, reg, runner.New(st, reg, runner.NewHub()))
	p := providerNamed(t, s, "opencode")
	if !p.Available || !p.DirectLogin || !strings.Contains(p.CLIRequirement, "optional") {
		t.Fatalf("bad provider metadata %+v", p)
	}
	claude := providerNamed(t, s, "claude")
	if claude.DirectLogin || !strings.Contains(claude.CLIRequirement, "required") {
		t.Fatal("Claude must require CLI")
	}
	response := do(t, s, "PUT", "/api/providers/claude/accounts/0/connection", map[string]string{"connection": "direct", "key": "secret"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatal("Claude accepted direct auth")
	}
	response.Body.Close()
	created := decode[accountInfo](t, do(t, s, "POST", "/api/providers/opencode/accounts", map[string]string{"alias": "Work"}))
	route := "/api/providers/opencode/accounts/" + itoa(created.ID) + "/connection"
	response = do(t, s, "PUT", route, map[string]string{"connection": "direct", "key": "private-go-key"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("save %d", response.StatusCode)
	}
	response.Body.Close()
	response = do(t, s, "GET", "/api/providers", nil)
	b, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(b), "private-go-key") {
		t.Fatal("API leaked key")
	}
	p = providerNamed(t, s, "opencode")
	if p.Accounts[0].SignedIn || !p.Accounts[1].SignedIn || p.Accounts[1].Connection != "direct" {
		t.Fatalf("isolation failed %+v", p.Accounts)
	}
	response = do(t, s, "PUT", route, map[string]string{"connection": "invalid", "key": "replacement"})
	if response.StatusCode != 400 {
		t.Fatal("accepted invalid mode")
	}
	response.Body.Close()
	response = do(t, s, "PUT", "/api/providers/claude/accounts/"+itoa(created.ID)+"/connection", map[string]string{"connection": "direct", "key": "replacement"})
	if response.StatusCode != 400 {
		t.Fatal("accepted mismatched account")
	}
	response.Body.Close()
}
