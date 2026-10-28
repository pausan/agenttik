package runner

import (
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

type forkingProvider struct{ *fakeProvider }

func (forkingProvider) CanFork() bool { return true }

func script(reply string) []agent.Event {
	return []agent.Event{
		{Type: agent.EventSessionStarted, ProviderSessionID: "prov-" + reply},
		{Type: agent.EventText, Text: reply},
		{Type: agent.EventDone, Usage: &agent.Usage{}},
	}
}

func turn(t *testing.T, r *Runner, id, prompt string) {
	t.Helper()
	if _, err := r.Send(id, prompt); err != nil {
		t.Fatalf("send %s: %v", id, err)
	}
	waitFor(t, func() bool { return !r.Running(id) }, "turn to finish")
}

func derived(t *testing.T, st *store.Store, parent *store.Session, id, parentID, originID string) *store.Session {
	t.Helper()
	s := &store.Session{ID: id, ProjectID: parent.ProjectID, Title: id, Provider: parent.Provider,
		AccountID: parent.AccountID, Model: parent.Model, Permission: parent.Permission,
		ParentID: parentID, AnchorQuote: "the passage", OriginID: originID}
	if parentID == "" {
		s.AnchorQuote = ""
	}
	if err := st.CreateSession(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// A provider that forks starts a thread as a copy of its conversation and
// tells it only which passage it is about; later turns resume the copy.
func TestThreadForksItsConversation(t *testing.T) {
	fp := &fakeProvider{script: script("main")}
	r, st, sess := setup(t, forkingProvider{fp})
	turn(t, r, sess.ID, "please look at x")

	thread := derived(t, st, sess, "thread-1", sess.ID, "")
	fp.script = script("thread")
	turn(t, r, thread.ID, "why?")
	if fp.lastReq.ForkFrom != "prov-main" || fp.lastReq.ProviderSessionID != "" {
		t.Fatalf("first thread turn should fork prov-main: %+v", fp.lastReq)
	}
	if !strings.Contains(fp.lastReq.Prompt, "<quote>\nthe passage\n</quote>") || !strings.HasSuffix(fp.lastReq.Prompt, "why?") ||
		strings.Contains(fp.lastReq.Prompt, "<conversation>") {
		t.Fatalf("prompt: %q", fp.lastReq.Prompt)
	}
	turn(t, r, thread.ID, "and then?")
	if fp.lastReq.ForkFrom != "" || fp.lastReq.ProviderSessionID != "prov-thread" || fp.lastReq.Prompt != "and then?" {
		t.Fatalf("second thread turn should resume the copy: %+v", fp.lastReq)
	}
	parent, _ := st.GetSession(sess.ID)
	if parent.ProviderSessionID != "prov-main" {
		t.Fatalf("parent conversation moved: %q", parent.ProviderSessionID)
	}

	task := derived(t, st, sess, "task-2", "", thread.ID)
	turn(t, r, task.ID, "carry on")
	if fp.lastReq.ForkFrom != "prov-thread" {
		t.Fatalf("spawned task should fork the thread: %+v", fp.lastReq)
	}
}

// Without forking, the transcript is given as text: for a thread its
// conversation, for a spawned task that conversation, the passage and the
// thread.
func TestThreadWithoutForkGetsTranscript(t *testing.T) {
	fp := &fakeProvider{script: script("main")}
	r, st, sess := setup(t, fp)
	turn(t, r, sess.ID, "please look at x")

	thread := derived(t, st, sess, "thread-1", sess.ID, "")
	fp.script = script("thread answer")
	turn(t, r, thread.ID, "why?")
	p := fp.lastReq.Prompt
	if fp.lastReq.ForkFrom != "" || !strings.Contains(p, "<conversation>") ||
		!strings.Contains(p, "User: please look at x\n\nAssistant: main") || !strings.Contains(p, "<quote>\nthe passage") {
		t.Fatalf("thread prompt: %q", p)
	}

	task := derived(t, st, sess, "task-2", "", thread.ID)
	turn(t, r, task.ID, "carry on")
	p = fp.lastReq.Prompt
	for _, want := range []string{"User: please look at x", "side thread about this passage", "User: why?", "Assistant: thread answer"} {
		if !strings.Contains(p, want) {
			t.Fatalf("task prompt lacks %q: %q", want, p)
		}
	}
	if strings.Contains(p, "<thread>") {
		t.Fatalf("a spawned task is not a thread: %q", p)
	}
}

// A model change the provider can resume keeps its thread and is given only
// the prompt. A reset thread — another provider or subscription — is handed
// the conversation and the project prompt once, and resumed after that.
func TestResetThreadIsHandedTheConversation(t *testing.T) {
	fp := &fakeProvider{script: script("main")}
	r, st, sess := setup(t, fp)
	if err := st.SetProjectPrompt(sess.ProjectID, "project rules"); err != nil {
		t.Fatal(err)
	}
	turn(t, r, sess.ID, "please look at x")

	if err := st.SetSessionModel(sess.ID, "fake", 0, "m2", "low", false); err != nil {
		t.Fatal(err)
	}
	turn(t, r, sess.ID, "go on")
	if fp.lastReq.ProviderSessionID != "prov-main" || fp.lastReq.Prompt != "go on" {
		t.Fatalf("a resumable model change should resume: %+v", fp.lastReq)
	}

	if err := st.SetSessionModel(sess.ID, "fake", 0, "m3", "low", true); err != nil {
		t.Fatal(err)
	}
	fp.script = script("other")
	turn(t, r, sess.ID, "and now?")
	p := fp.lastReq.Prompt
	if fp.lastReq.ProviderSessionID != "" || !strings.HasPrefix(p, "project rules\n\n<conversation>") ||
		!strings.Contains(p, "User: please look at x\n\nAssistant: main\n\nUser: go on\n\nAssistant: main") ||
		!strings.HasSuffix(p, "</conversation>\n\nand now?") {
		t.Fatalf("reset thread prompt: %+v", fp.lastReq)
	}

	turn(t, r, sess.ID, "thanks")
	if fp.lastReq.ProviderSessionID != "prov-other" || fp.lastReq.Prompt != "thanks" {
		t.Fatalf("the new thread should be resumed: %+v", fp.lastReq)
	}
}
