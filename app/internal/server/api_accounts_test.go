package server

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

// twoLoginProvider holds its login in a directory, as the real CLIs do.
// singleLoginProvider does not, which is the other half of the contract: the
// API has to refuse a second subscription for it rather than store a path
// nothing will read.
type twoLoginProvider struct{ singleLoginProvider }

type singleLoginProvider struct{ name string }

func (p singleLoginProvider) Name() string        { return p.name }
func (p singleLoginProvider) DisplayName() string { return p.name }
func (p singleLoginProvider) Models() []agent.Model {
	return []agent.Model{{ID: "m1", Label: "M1"}}
}
func (p singleLoginProvider) Efforts() []string { return []string{"low"} }
func (p singleLoginProvider) Available() error  { return nil }
func (p singleLoginProvider) Run(context.Context, agent.TurnRequest) (<-chan agent.Event, error) {
	events := make(chan agent.Event)
	close(events)
	return events, nil
}

func (p twoLoginProvider) DefaultHome() string { return "/tmp/" + p.name }
func (p twoLoginProvider) AccountStatus(home string) agent.AccountStatus {
	return agent.AccountStatus{SignedIn: home != "", Detail: home}
}
func (p twoLoginProvider) LoginCommand(home string) agent.LoginCommand {
	return agent.LoginCommand{Args: []string{p.name, "login"}}
}

func accountServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	reg := agent.NewRegistry(
		twoLoginProvider{singleLoginProvider{name: "multi"}},
		singleLoginProvider{name: "single"},
	)
	return New(st, reg, runner.New(st, reg, runner.NewHub())), st
}

func providerNamed(t *testing.T, s *Server, name string) providerInfo {
	t.Helper()
	for _, p := range decode[[]providerInfo](t, do(t, s, "GET", "/api/providers", nil)) {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("provider %q missing from the list", name)
	return providerInfo{}
}

func TestProvidersAlwaysOfferTheSystemAccount(t *testing.T) {
	s, _ := accountServer(t)

	multi := providerNamed(t, s, "multi")
	if !multi.MultiAccount {
		t.Error("a provider that can hold a second login says it cannot")
	}
	if len(multi.Accounts) != 1 || !multi.Accounts[0].System || !multi.Accounts[0].IsDefault {
		t.Fatalf("accounts = %+v, want the system account alone and default", multi.Accounts)
	}
	if multi.Accounts[0].Home != "/tmp/multi" {
		t.Errorf("system home = %q, want the CLI's own directory", multi.Accounts[0].Home)
	}

	// A provider whose login cannot be moved still offers one account, so
	// every picker has the same shape to draw.
	single := providerNamed(t, s, "single")
	if single.MultiAccount || len(single.Accounts) != 1 || !single.Accounts[0].System {
		t.Errorf("single-login provider = %+v", single)
	}
}

func TestAddingASubscriptionAndSwappingToIt(t *testing.T) {
	s, st := accountServer(t)

	resp := do(t, s, "POST", "/api/providers/multi/accounts", map[string]any{"alias": "Work"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d, want 201", resp.StatusCode)
	}
	created := decode[accountInfo](t, resp)
	// A home nobody named is one the app manages, beside the database.
	if created.Home != filepath.Join(st.Dir(), "accounts", "multi", "work") {
		t.Errorf("home = %q, want a managed directory", created.Home)
	}

	// Until it is chosen, the machine's own login is still what new tasks get.
	if id, _ := st.DefaultAccount("multi"); id != store.SystemAccount {
		t.Errorf("adding a subscription changed the default to %d", id)
	}
	if resp := do(t, s, "PUT", "/api/providers/multi/account",
		map[string]any{"account_id": created.ID}); resp.StatusCode != http.StatusOK {
		t.Fatalf("set default = %d", resp.StatusCode)
	}

	multi := providerNamed(t, s, "multi")
	if len(multi.Accounts) != 2 {
		t.Fatalf("accounts = %+v, want two", multi.Accounts)
	}
	if multi.Accounts[0].IsDefault || !multi.Accounts[1].IsDefault {
		t.Errorf("the default did not move to the new subscription: %+v", multi.Accounts)
	}

	// And a task started now lands on it without being told to.
	project, err := st.CreateProject("p", t.TempDir())
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	session := decode[store.Session](t, do(t, s, "POST", "/api/sessions",
		map[string]any{"project_id": project.ID, "provider": "multi"}))
	if session.AccountID != created.ID {
		t.Errorf("new task is on account %d, want the default %d", session.AccountID, created.ID)
	}
}

func TestSubscriptionsCannotCrossProviders(t *testing.T) {
	s, _ := accountServer(t)
	created := decode[accountInfo](t, do(t, s, "POST", "/api/providers/multi/accounts",
		map[string]any{"alias": "Work"}))

	// The alias belongs to one provider, and so does the id. Pairing them
	// wrongly would run a prompt on another provider's allowance, so every
	// route that takes both refuses the mismatch.
	id := itoa(created.ID)
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/providers/single/accounts/" + id + "/usage", nil},
		{"PATCH", "/api/providers/single/accounts/" + id, map[string]any{"alias": "Other"}},
		{"POST", "/api/providers/single/accounts/" + id + "/login", nil},
		{"DELETE", "/api/providers/single/accounts/" + id, nil},
		{"PUT", "/api/providers/single/account", map[string]any{"account_id": created.ID}},
	} {
		resp := do(t, s, call.method, call.path, call.body)
		if resp.StatusCode < 400 {
			t.Errorf("%s %s was accepted (%d)", call.method, call.path, resp.StatusCode)
		}
	}
	if resp := do(t, s, "POST", "/api/providers/single/accounts",
		map[string]any{"alias": "Work"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a provider that cannot hold a second login accepted one: %d", resp.StatusCode)
	}
}

func TestSubscriptionAliasesAreRequiredAndUnique(t *testing.T) {
	s, _ := accountServer(t)
	for _, body := range []map[string]any{
		{"alias": ""},
		{"alias": "  "},
		{"alias": "System"}, // reserved: it is what the CLI's own login is called
		{"alias": "Work", "home": "relative/path"},
	} {
		if resp := do(t, s, "POST", "/api/providers/multi/accounts", body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%v was accepted (%d)", body, resp.StatusCode)
		}
	}
	if resp := do(t, s, "POST", "/api/providers/multi/accounts",
		map[string]any{"alias": "Work"}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	if resp := do(t, s, "POST", "/api/providers/multi/accounts",
		map[string]any{"alias": "Work"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a duplicate alias was accepted: %d", resp.StatusCode)
	}
}

func TestSwappingSubscriptionStartsANewThread(t *testing.T) {
	s, st := accountServer(t)
	created := decode[accountInfo](t, do(t, s, "POST", "/api/providers/multi/accounts",
		map[string]any{"alias": "Work"}))
	project, err := st.CreateProject("p", t.TempDir())
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	session := decode[store.Session](t, do(t, s, "POST", "/api/sessions",
		map[string]any{"project_id": project.ID, "provider": "multi"}))
	if err := st.SetProviderSessionID(session.ID, "thread-1"); err != nil {
		t.Fatalf("set thread: %v", err)
	}

	updated := decode[store.Session](t, do(t, s, "PATCH", "/api/sessions/"+session.ID,
		map[string]any{"account_id": created.ID}))
	if updated.AccountID != created.ID {
		t.Errorf("account = %d, want %d", updated.AccountID, created.ID)
	}
	// The thread belongs to the account that opened it; the other cannot
	// resume it, so the swap clears it exactly as a provider switch does.
	if updated.ProviderSessionID != "" {
		t.Errorf("thread %q survived the swap", updated.ProviderSessionID)
	}
}

func TestRemovedSubscriptionStopsItsTasksRatherThanMovingThem(t *testing.T) {
	s, st := accountServer(t)
	created := decode[accountInfo](t, do(t, s, "POST", "/api/providers/multi/accounts",
		map[string]any{"alias": "Work"}))
	project, err := st.CreateProject("p", t.TempDir())
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	session := decode[store.Session](t, do(t, s, "POST", "/api/sessions",
		map[string]any{"project_id": project.ID, "provider": "multi", "account_id": created.ID}))

	usage := decode[map[string]int64](t, do(t, s, "GET",
		"/api/providers/multi/accounts/"+itoa(created.ID)+"/usage", nil))
	if usage["sessions"] != 1 {
		t.Errorf("usage said %d tasks before removing, want 1", usage["sessions"])
	}
	if resp := do(t, s, "DELETE", "/api/providers/multi/accounts/"+itoa(created.ID), nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d", resp.StatusCode)
	}

	// The task keeps pointing at what is gone and refuses to run, rather than
	// quietly spending the machine's own allowance.
	kept, err := st.GetSession(session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if kept.AccountID != created.ID {
		t.Fatalf("task moved to account %d on its own", kept.AccountID)
	}
	// A bad request rather than a server fault: picking another subscription
	// for the task is the fix, and the message says as much.
	resp := do(t, s, "POST", "/api/sessions/"+session.ID+"/messages", map[string]any{"prompt": "hi"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a prompt on a removed subscription answered %d, want 400", resp.StatusCode)
	}
}

func TestSubscriptionLimitsAreAskedPerAccount(t *testing.T) {
	s, _ := accountServer(t)
	created := decode[accountInfo](t, do(t, s, "POST", "/api/providers/multi/accounts",
		map[string]any{"alias": "Work"}))
	// An id belonging to another provider is refused here too: these bars sit
	// beside the model picker, and the wrong ones read as the right ones.
	if resp := do(t, s, "GET",
		"/api/providers/single/subscription-limits?account="+itoa(created.ID), nil); resp.StatusCode == http.StatusOK {
		t.Error("limits were served for another provider's subscription")
	}
	if resp := do(t, s, "GET",
		"/api/providers/multi/subscription-limits?account="+itoa(created.ID), nil); resp.StatusCode != http.StatusOK {
		t.Errorf("limits for its own subscription = %d", resp.StatusCode)
	}
}
