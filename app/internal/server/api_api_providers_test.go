package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/apiprovider"
	"github.com/pausan/agenttik/app/internal/runner"
	"github.com/pausan/agenttik/app/internal/store"
)

func apiProviderServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-only-key" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/models" {
			fmt.Fprint(w, `{"data":[{"id":"gpt-test","name":"Test model"}]}`)
			return
		}
		if r.URL.Path != "/chat/completions" {
			t.Error(r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		messages, _ := body["messages"].([]any)
		if len(messages) > 0 && messages[len(messages)-1].(map[string]any)["content"] == "hold for cancellation" {
			_, _ = io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
			return
		}
		if body["model"] != "gpt-test" {
			t.Error("wrong model", body["model"])
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Completed API task.\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":4}}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(upstream.Close)
	st, err := store.Open(filepath.Join(t.TempDir(), "agenttik.db"))
	if err != nil {
		t.Fatal(err)
	}
	provider := apiprovider.New(apiprovider.Definition{ID: "openai", Label: "OpenAI", BaseURL: upstream.URL, Protocol: "chat"}, st.Dir())
	reg := agent.NewRegistry(provider)
	run := runner.New(st, reg, runner.NewHub())
	s := New(st, reg, run)
	t.Cleanup(func() { s.Shutdown(); run.Shutdown(); st.Close() })
	return s, st
}
func TestAPIProviderKeyLifecycleAndTask(t *testing.T) {
	s, st := apiProviderServer(t)
	p := providerNamed(t, s, "api-openai")
	if p.Kind != "api" || p.Available || len(p.Models) != 0 || p.API.KeySet {
		t.Fatal(p)
	}
	project, err := st.CreateProject("api project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"project_id": project.ID, "provider": "api-openai"}
	if resp := do(t, s, "POST", "/api/sessions", body); resp.StatusCode != 400 {
		t.Fatal("disabled default model accepted", resp.StatusCode)
	}
	resp := do(t, s, "PUT", "/api/providers/api-openai/api", map[string]any{"key": "test-only-key", "enabled": true})
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 || strings.Contains(string(data), "test-only-key") {
		t.Fatal("unsafe setup response", resp.StatusCode, err)
	}
	p = providerNamed(t, s, "api-openai")
	if !p.Available || len(p.Models) != 1 || p.API == nil || !p.API.Enabled {
		t.Fatal(p)
	}
	session := decode[store.Session](t, do(t, s, "POST", "/api/sessions", body))
	if session.Model != "gpt-test" || session.Provider != "api-openai" {
		t.Fatal(session)
	}
	if _, err := s.runner.Send(session.ID, "do a task"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for s.runner.Running(session.ID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s.runner.Running(session.ID) {
		t.Fatal("task did not finish")
	}
	turns, err := st.ListTurns(session.ID)
	if err != nil || len(turns) != 1 || turns[0].Status != "ok" || turns[0].InputTokens != 12 {
		t.Fatal(turns, err)
	}
	if resp := do(t, s, "PUT", "/api/providers/api-openai/api", map[string]any{"enabled": false}); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	p = providerNamed(t, s, "api-openai")
	if p.Available || len(p.Models) != 0 || !p.API.KeySet {
		t.Fatal("disable failed", p)
	}
	if _, err := s.runner.Send(session.ID, "must not run"); err == nil {
		t.Fatal("disabled connection ran")
	}
	if resp := do(t, s, "DELETE", "/api/providers/api-openai/api", nil); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	if providerNamed(t, s, "api-openai").API.KeySet {
		t.Fatal("key retained")
	}
}

func TestAPIConnectionsAreSharedAcrossProfiles(t *testing.T) {
	s, _ := apiProviderServer(t)
	if err := s.EnableProfiles(false); err != nil {
		t.Fatal(err)
	}
	resp := do(t, s, "POST", "/api/profiles", map[string]string{"name": "Work"})
	profile := decode[Profile](t, resp)
	if profile.ID == "" {
		t.Fatal("no profile", resp.StatusCode)
	}
	if resp := do(t, s, "PUT", "/api/providers/api-openai/api", map[string]any{"key": "test-only-key", "enabled": true}); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	rows := decode[[]providerInfo](t, do(t, s, "GET", "/api/providers?profile="+profile.ID, nil))
	if len(rows) != 1 || !rows[0].Available || !rows[0].API.KeySet || len(rows[0].Models) == 0 {
		t.Fatal("profile did not inherit connection", rows)
	}
	if resp := do(t, s, "PUT", "/api/providers/api-openai/api?profile="+profile.ID, map[string]any{"key": "test-only-key", "enabled": true}); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	if resp := do(t, s, "DELETE", "/api/providers/api-openai/api", nil); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	rows = decode[[]providerInfo](t, do(t, s, "GET", "/api/providers?profile="+profile.ID, nil))
	if rows[0].Available || rows[0].API.KeySet {
		t.Fatal("removal did not disable other profile")
	}
}

type signedOutCatalogProvider struct{ twoLoginProvider }

func (p signedOutCatalogProvider) AccountStatus(string) agent.AccountStatus {
	return agent.AccountStatus{}
}
func TestSignedOutSubscriptionHasNoModels(t *testing.T) {
	s, st := newTestServer(t)
	s.registry = agent.NewRegistry(signedOutCatalogProvider{twoLoginProvider{singleLoginProvider{name: "signed-out"}}})
	if err := st.CreateAccount(&store.Account{Provider: "signed-out", Alias: "Work", Home: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	p := providerNamed(t, s, "signed-out")
	if !p.Available || len(p.Models) != 0 || len(p.Accounts) != 2 {
		t.Fatal("subscription discovery did not keep setup while hiding models", p)
	}
}

func TestAPITaskPermissionPersistsAndKeepsConversation(t *testing.T) {
	s, st := apiProviderServer(t)
	if resp := do(t, s, "PUT", "/api/providers/api-openai/api", map[string]any{"key": "test-only-key", "enabled": true}); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	project, err := st.CreateProject("permissions", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session := decode[store.Session](t, do(t, s, "POST", "/api/sessions", map[string]any{"project_id": project.ID, "provider": "api-openai"}))
	if err := st.SetProviderSessionID(session.ID, "keep-this-conversation"); err != nil {
		t.Fatal(err)
	}
	path := "/api/sessions/" + session.ID
	if resp := do(t, s, "PATCH", path, map[string]string{"permission": "invalid"}); resp.StatusCode != 400 {
		t.Fatal("invalid permission accepted")
	}
	full := decode[store.Session](t, do(t, s, "PATCH", path, map[string]string{"permission": "full"}))
	if full.Permission != "full" || full.ProviderSessionID != "keep-this-conversation" {
		t.Fatal(full)
	}
	stored, err := st.GetSession(session.ID)
	if err != nil || stored.Permission != "full" {
		t.Fatal(stored, err)
	}
	if err := st.SetProviderSessionID(session.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.runner.Send(session.ID, "hold for cancellation"); err != nil {
		t.Fatal(err)
	}
	defer s.runner.Stop(session.ID)
	if resp := do(t, s, "PATCH", path, map[string]string{"permission": "plan"}); resp.StatusCode != 400 {
		t.Fatal("running task changed permission", resp.StatusCode)
	}
	stored, _ = st.GetSession(session.ID)
	if stored.Permission != "full" {
		t.Fatal("failed permission change was written")
	}
}
