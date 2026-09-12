package runner

import (
	"os"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/orchestrator"
	"github.com/pausan/agenttik/app/internal/store"
)

func TestOrchestratorReceivesFreshConnectionOnEveryTurn(t *testing.T) {
	fp := &fakeProvider{script: []agent.Event{
		{Type: agent.EventSessionStarted, ProviderSessionID: "provider-thread"},
		{Type: agent.EventDone},
	}}
	r, st, ordinary := setup(t, fp)
	if _, err := r.Send(ordinary.ID, "ordinary question"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !r.Running(ordinary.ID) }, "ordinary turn")
	if fp.lastReq.Prompt != "ordinary question" {
		t.Fatal("orchestrator context reached an ordinary project")
	}
	if err := st.SetOrchestratorEnabled(true); err != nil {
		t.Fatal(err)
	}
	cfg, _ := st.GetOrchestratorConfig()
	if err := st.SetProjectPrompt(cfg.ProjectID, "Custom orchestration rules"); err != nil {
		t.Fatal(err)
	}
	sess := &store.Session{ID: "orchestrator-task", ProjectID: cfg.ProjectID, Provider: "fake", Model: "m1"}
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	executable, _ := os.Executable()
	connection := orchestrator.Context(executable, st.Dir(), sess.ID, cfg.ProjectID)
	for i, prompt := range []string{"Which tasks are running?", "And now?"} {
		if _, err := r.Send(sess.ID, prompt); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return !r.Running(sess.ID) }, "orchestrator turn")
		if !strings.HasPrefix(fp.lastReq.Prompt, connection) || !strings.HasSuffix(fp.lastReq.Prompt, prompt) || fp.lastReq.WorkDir != cfg.Path {
			t.Fatalf("turn request = %+v", fp.lastReq)
		}
		if strings.Contains(fp.lastReq.Prompt, "Custom orchestration rules") != (i == 0) {
			t.Fatal("standing prompt must be injected only on the first turn")
		}
		if err := st.ResetOrchestratorPrompt(); err != nil {
			t.Fatal(err)
		}
	}
	stored, _ := st.GetSession(sess.ID)
	if stored.ProjectPrompt != "Custom orchestration rules" {
		t.Fatal("reset rewrote an existing conversation's instructions")
	}
	messages, _ := st.ListMessages(sess.ID)
	for _, message := range messages {
		if strings.Contains(message.Content, "<agenttik-runtime>") {
			t.Fatal("runtime connection was saved as a user message")
		}
	}
	sess.ID = "new-orchestrator-task"
	if err := st.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Send(sess.ID, "New conversation"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return !r.Running(sess.ID) }, "new orchestrator turn")
	if !strings.Contains(fp.lastReq.Prompt, orchestrator.DefaultPrompt) {
		t.Fatal("new conversation did not get the reset default")
	}
}
