package fake

import (
	"context"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestCutDirective(t *testing.T) {
	cases := []struct {
		line string
		name string
		rest string
		ok   bool
	}{
		{"@wait 300", "wait", "300", true},
		{"  @tool Bash echo hi", "tool", "Bash echo hi", true},
		{"@error rate limit reached", "error", "rate limit reached", true},
		{"@bogus x", "", "", false}, // not a known directive: kept as text
		{"plain reply line", "", "", false},
		{"@", "", "", false},
	}
	for _, c := range cases {
		name, rest, ok := cutDirective(c.line)
		if name != c.name || rest != c.rest || ok != c.ok {
			t.Errorf("cutDirective(%q) = %q, %q, %v; want %q, %q, %v",
				c.line, name, rest, ok, c.name, c.rest, c.ok)
		}
	}
}

func TestWrapped(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		tag    string
		want   string
	}{
		{"wrapped prompt", "Name the task.\n\n<user-request>\nfix the flaky test\n</user-request>", "user-request", "fix the flaky test"},
		{"only the first line of a multi-line request", "<user-request>\nfix it\nand also this\n</user-request>", "user-request", "fix it"},
		{"no wrapper at all falls back to the whole title prompt", "just a plain prompt", "user-request", "just a plain prompt"},
		{"a reply to summarise", "Say what it came to.\n\n<agent-reply>\nDone: the test passes\n</agent-reply>", "agent-reply", "Done: the test passes"},
		{"a title request carries no reply", "<user-request>\nfix it\n</user-request>", "agent-reply", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wrapped(c.prompt, c.tag); got != c.want {
				t.Errorf("wrapped(%q, %q) = %q, want %q", c.prompt, c.tag, got, c.want)
			}
		})
	}
}

func TestEnabledReadsEnvVar(t *testing.T) {
	t.Setenv("AGENTTIK_FAKE_PROVIDER", "")
	if Enabled() {
		t.Error("Enabled() with an empty env var, want false")
	}
	t.Setenv("AGENTTIK_FAKE_PROVIDER", "1")
	if !Enabled() {
		t.Error("Enabled() with AGENTTIK_FAKE_PROVIDER=1, want true")
	}
}

// @open keeps the turn open while a background task runs and takes prompts
// meanwhile; the reply that ends the last task ends the turn.
func TestOpenTurnTakesPromptsUntilTasksEnd(t *testing.T) {
	events, err := New().Run(context.Background(), agent.TurnRequest{
		Prompt: "@open\n@background start b1 build\nStarted"})
	if err != nil {
		t.Fatal(err)
	}
	next := func(want agent.EventType) agent.Event {
		t.Helper()
		for ev := range events {
			if ev.Type == want {
				return ev
			}
		}
		t.Fatalf("no %s event", want)
		return agent.Event{}
	}
	done := next(agent.EventDone)
	if done.FollowUp == nil {
		t.Fatal("a reply with a task running should leave the turn open")
	}
	if err := done.FollowUp.Send("@background end b1 completed built\nFinished"); err != nil {
		t.Fatal(err)
	}
	next(agent.EventFollowUp)
	if done = next(agent.EventDone); done.FollowUp != nil {
		t.Fatal("the turn stayed open with no task running")
	}
	if _, open := <-events; open {
		t.Fatal("events continued after the last reply")
	}
}
