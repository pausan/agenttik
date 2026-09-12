package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestBuildArgs(t *testing.T) {
	got := strings.Join(buildArgs(agent.TurnRequest{
		WorkDir: "/tmp/p", Model: "gpt-5-codex", Effort: "high",
		Permission: agent.PermissionWorkspace}), " ")
	for _, want := range []string{"exec", "--json", "--cd /tmp/p",
		"--model gpt-5-codex", "model_reasoning_effort=high", "--sandbox workspace-write"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
}

func TestBuildArgsResume(t *testing.T) {
	got := strings.Join(buildArgs(agent.TurnRequest{ProviderSessionID: "s1", WorkDir: "/tmp/p"}), " ")
	if !strings.HasPrefix(got, "exec resume s1") {
		t.Errorf("args %q should start with 'exec resume s1'", got)
	}
}

func TestSandboxMapping(t *testing.T) {
	cases := map[agent.Permission]string{
		agent.PermissionPlan:      "read-only",
		agent.PermissionWorkspace: "workspace-write",
		agent.PermissionFull:      "danger-full-access",
	}
	for perm, want := range cases {
		if got := sandboxFlag(perm); got != want {
			t.Errorf("permission %q -> %q, want %q", perm, got, want)
		}
	}
}

func TestExecTurnUsageIsNotContext(t *testing.T) {
	out := make(chan agent.Event, 4)
	handleLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":2500000,`+
		`"cached_input_tokens":2000000,"output_tokens":500}}`), out)
	close(out)

	var got []agent.Event
	for event := range out {
		got = append(got, event)
	}
	if len(got) != 1 || got[0].Type != agent.EventDone {
		t.Fatalf("got %+v", got)
	}
	if got[0].Usage.ContextTokens != 0 || got[0].Usage.InputTokens != 2500000 {
		t.Errorf("aggregate turn usage was mislabeled as context: %+v", got[0].Usage)
	}
}

func TestAppServerUsageUsesRootLastAndIncludesSubagents(t *testing.T) {
	tracker := appUsageTracker{
		rootID: "root", threads: make(map[string]*appTrackedUsage),
		subagents: make(map[string]struct{}),
	}
	window := int64(1_050_000)

	// The first root update is from a resumed conversation. Its historical
	// 100 input tokens are excluded by total-last.
	context, gotWindow, root := tracker.update("root", appThreadTokenUsage{
		Total:              appTokenBreakdown{InputTokens: 120, OutputTokens: 15, CachedInputTokens: 50},
		Last:               appTokenBreakdown{InputTokens: 20, OutputTokens: 5, CachedInputTokens: 20},
		ModelContextWindow: &window,
	})
	if !root || context != 20 || gotWindow != window {
		t.Fatalf("first root update = context %d, window %d, root %v", context, gotWindow, root)
	}
	tracker.update("root", appThreadTokenUsage{
		Total:              appTokenBreakdown{InputTokens: 150, OutputTokens: 25, CachedInputTokens: 70},
		Last:               appTokenBreakdown{InputTokens: 30, OutputTokens: 10, CachedInputTokens: 20},
		ModelContextWindow: &window,
	})
	tracker.update("child", appThreadTokenUsage{
		Total: appTokenBreakdown{InputTokens: 40, OutputTokens: 8, CachedInputTokens: 12},
		Last:  appTokenBreakdown{InputTokens: 40, OutputTokens: 8, CachedInputTokens: 12},
	})

	got := tracker.usage()
	if got.ContextTokens != 30 || got.ContextWindow != window {
		t.Errorf("main context = %d/%d, want 30/%d", got.ContextTokens, got.ContextWindow, window)
	}
	if got.MainInputTokens != 50 || got.MainOutputTokens != 15 ||
		got.SubagentInputTokens != 40 || got.SubagentOutputTokens != 8 ||
		got.InputTokens != 90 || got.OutputTokens != 23 ||
		got.MainCacheReadTokens != 40 || got.SubagentCacheReadTokens != 12 ||
		got.SubagentCount != 1 || !got.UsageBreakdown {
		t.Errorf("usage = %+v", got)
	}
}

func TestAppServerResumeRequestSkipsHistory(t *testing.T) {
	method, params := threadRequest(agent.TurnRequest{
		ProviderSessionID: "thread-1", WorkDir: "/tmp/p",
		Permission: agent.PermissionWorkspace,
	})
	if method != "thread/resume" || params["threadId"] != "thread-1" ||
		params["excludeTurns"] != true || params["sandbox"] != "workspace-write" {
		t.Errorf("request = %s %+v", method, params)
	}
}

func TestAppServerNotificationsKeepChildOutOfRootContext(t *testing.T) {
	out := make(chan agent.Event, 8)
	run := appServerRun{
		out: out, rootID: "root",
		tracker: appUsageTracker{
			rootID: "root", threads: make(map[string]*appTrackedUsage),
			subagents: make(map[string]struct{}),
		},
		streamed: make(map[string]bool), activeTurns: make(map[string]string),
	}
	handle := func(method, params string) bool {
		t.Helper()
		done, err := run.handleNotification(method, json.RawMessage(params))
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		return done
	}

	// This is a persisted total announced while the root is loading. It is
	// retained as the baseline, not charged to the new task.
	handle("thread/tokenUsage/updated", `{"threadId":"root","turnId":"old",`+
		`"tokenUsage":{"total":{"inputTokens":100,"outputTokens":10},`+
		`"last":{"inputTokens":20,"outputTokens":2},"modelContextWindow":1000}}`)
	handle("turn/started", `{"threadId":"root","turn":{"id":"root-turn"}}`)
	handle("thread/tokenUsage/updated", `{"threadId":"root","turnId":"root-turn",`+
		`"tokenUsage":{"total":{"inputTokens":130,"outputTokens":15},`+
		`"last":{"inputTokens":30,"outputTokens":5},"modelContextWindow":1000}}`)
	handle("thread/started", `{"thread":{"id":"child","parentThreadId":"root"}}`)
	handle("turn/started", `{"threadId":"child","turn":{"id":"child-turn"}}`)
	handle("thread/tokenUsage/updated", `{"threadId":"child","turnId":"child-turn",`+
		`"tokenUsage":{"total":{"inputTokens":40,"outputTokens":8},`+
		`"last":{"inputTokens":40,"outputTokens":8},"modelContextWindow":500}}`)
	if !handle("turn/completed", `{"threadId":"root","turn":{"id":"root-turn","status":"completed"}}`) {
		t.Fatal("root completion did not finish the stream")
	}

	close(out)
	var got []agent.Event
	for event := range out {
		got = append(got, event)
	}
	if len(got) != 2 || got[0].Type != agent.EventUsage || got[1].Type != agent.EventDone {
		t.Fatalf("got %+v", got)
	}
	if got[0].Usage.ContextTokens != 30 {
		t.Errorf("live main context = %d, want 30", got[0].Usage.ContextTokens)
	}
	usage := got[1].Usage
	if usage.ContextTokens != 30 || usage.MainInputTokens != 30 ||
		usage.SubagentInputTokens != 40 || usage.InputTokens != 70 ||
		usage.SubagentCount != 1 {
		t.Errorf("done usage = %+v", usage)
	}
}
