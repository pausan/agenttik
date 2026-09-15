package server

import (
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/claudecode"
	"github.com/pausan/agenttik/app/internal/agent/codex"
	"github.com/pausan/agenttik/app/internal/agent/fake"
	"github.com/pausan/agenttik/app/internal/store"
)

type availableModelProvider struct{ agent.Provider }

func (p availableModelProvider) SmallModel() (string, string) {
	return p.Provider.(agent.SmallModel).SmallModel()
}
func (p availableModelProvider) Available() error { return nil }

func TestActionModelDefaultsAndOverrides(t *testing.T) {
	s, st := newTestServer(t)
	s.registry = agent.NewRegistry(availableModelProvider{claudecode.New()}, availableModelProvider{codex.New()}, fake.New())
	if got := s.defaultActionModel("merge"); got.Provider != "codex" || got.Model != "gpt-6-astra" || got.Effort != "high" {
		t.Fatal(got)
	}
	if got := s.defaultActionModel("commit_message"); got.Model != "gpt-5.6-luna" || got.Effort != "none" {
		t.Fatal(got)
	}
	s.registry = agent.NewRegistry(availableModelProvider{claudecode.New()}, fake.New())
	if got := s.defaultActionModel("merge"); got.Model != "opus" || got.Effort != "high" {
		t.Fatal(got)
	}
	choice := store.ActionModel{Provider: "fake", Model: "fake-careful", Effort: "high"}
	res := do(t, s, "PUT", "/api/action-models/commit_message", choice)
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	got, err := s.actionModel("commit_message")
	if err != nil || got != choice {
		t.Fatalf("%+v %v", got, err)
	}
	choices, err := st.ActionModels()
	if err != nil || choices["commit_message"] != choice {
		t.Fatalf("%+v %v", choices, err)
	}
	for _, bad := range []store.ActionModel{
		{Provider: "missing", Model: "x"}, {Provider: "fake", Model: "missing"},
		{Provider: "fake", Model: "fake-quick", Effort: "max"}, {Provider: "fake", Model: "fake-quick", AccountID: 999},
	} {
		if res := do(t, s, "PUT", "/api/action-models/merge", bad); res.StatusCode < 400 {
			t.Fatal(bad)
		}
	}
	if res := do(t, s, "PUT", "/api/action-models/missing", choice); res.StatusCode != 400 {
		t.Fatal(res.StatusCode)
	}
	if res := do(t, s, "PUT", "/api/action-models/commit_message", map[string]string{}); res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	if got, _ := s.actionModel("commit_message"); got.Model != "haiku" {
		t.Fatal(got)
	}
	if err := st.SetActionModel("merge", &choice); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetPreferences(); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ActionModels(); len(got) != 0 {
		t.Fatal(got)
	}
}

// A bundled direct provider may be available without any configured login.
// It must not take precedence over a subscription that can actually run.
type signedOutProvider struct{ fake.Provider }

func (p signedOutProvider) Name() string                             { return "codex" }
func (p signedOutProvider) AccountStatus(string) agent.AccountStatus { return agent.AccountStatus{} }
func TestActionDefaultSkipsSignedOutSubscription(t *testing.T) {
	s, _ := newTestServer(t)
	s.registry = agent.NewRegistry(&signedOutProvider{}, fake.New())
	if got := s.defaultActionModel("commit_message"); got.Provider != "fake" {
		t.Fatal(got)
	}
}
