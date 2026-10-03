package codex

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestBuildArgs(t *testing.T) {
	got := strings.Join(buildArgs(agent.TurnRequest{
		WorkDir: "/tmp/p", Model: "gpt-5-codex", Effort: "high",
		Permission: agent.PermissionWorkspace}), " ")
	for _, want := range []string{"exec", "--json", "--cd /tmp/p",
		"--model gpt-5-codex", "model_reasoning_effort=high", "--dangerously-bypass-approvals-and-sandbox"} {
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
	if !strings.Contains(got, "--dangerously-bypass-approvals-and-sandbox") {
		t.Errorf("resume should disable the sandbox by default: %q", got)
	}
}

func TestSandboxMapping(t *testing.T) {
	cases := map[agent.Permission]string{
		agent.PermissionPlan:      "read-only",
		agent.PermissionWorkspace: "danger-full-access",
		agent.PermissionFull:      "danger-full-access",
		"":                        "danger-full-access",
		agent.Permission("junk"):  "danger-full-access",
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
		params["excludeTurns"] != true || params["sandbox"] != "danger-full-access" ||
		params["approvalPolicy"] != "on-request" {
		t.Errorf("request = %s %+v", method, params)
	}
}

func TestAppServerDefaultPermissions(t *testing.T) {
	method, params := threadRequest(agent.TurnRequest{WorkDir: "/tmp/p"})
	if method != "thread/start" || params["sandbox"] != "danger-full-access" ||
		params["approvalPolicy"] != "on-request" {
		t.Errorf("request = %s %+v", method, params)
	}
}

func TestApprovalPolicyAsksOnlyForWorkspace(t *testing.T) {
	for perm, want := range map[agent.Permission]string{
		agent.PermissionPlan: "never", agent.PermissionWorkspace: "on-request", agent.PermissionFull: "never",
	} {
		turn := turnRequest(agent.TurnRequest{Permission: perm}, "t")
		if turn["approvalPolicy"] != want {
			t.Errorf("%s: policy %v, want %s", perm, turn["approvalPolicy"], want)
		}
		if reviewer, set := turn["approvalsReviewer"]; (want == "on-request") != set || (set && reviewer != "user") {
			t.Errorf("%s: reviewer %v", perm, turn["approvalsReviewer"])
		}
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
	if len(got) != 3 || got[0].Type != agent.EventUsage || got[1].Type != agent.EventUsage || got[2].Type != agent.EventDone {
		t.Fatalf("got %+v", got)
	}
	if got[0].Usage.ContextTokens != 30 || got[0].Usage.InputTokens != 30 ||
		got[1].Usage.ContextTokens != 30 || got[1].Usage.InputTokens != 70 {
		t.Errorf("live usage = %+v, %+v", got[0].Usage, got[1].Usage)
	}
	usage := got[2].Usage
	if usage.ContextTokens != 30 || usage.MainInputTokens != 30 ||
		usage.SubagentInputTokens != 40 || usage.InputTokens != 70 ||
		usage.SubagentCount != 1 {
		t.Errorf("done usage = %+v", usage)
	}
}

// A command still running when the agent moves on, as unified exec does once
// a command outlives its yield time, is a background task until it exits.
// Calls started together stay foreground.
func TestAppServerCommandLeftRunningIsBackground(t *testing.T) {
	clock := time.UnixMilli(1_000_000)
	now = func() time.Time { return clock }
	defer func() { now = time.Now }()
	out := make(chan agent.Event, 32)
	run := appServerRun{out: out, rootID: "t"}
	notify := func(method, params string) {
		t.Helper()
		if _, err := run.handleNotification(method, json.RawMessage(params)); err != nil {
			t.Fatal(err)
		}
	}
	background := func() []agent.BackgroundTask {
		var got []agent.BackgroundTask
		for {
			select {
			case ev := <-out:
				if ev.Type == agent.EventBackground {
					got = append(got, *ev.Background)
				}
			default:
				return got
			}
		}
	}

	notify("item/started", `{"threadId":"t","item":{"type":"commandExecution","id":"c1","command":"make test","status":"inProgress"}}`)
	notify("item/started", `{"threadId":"t","item":{"type":"commandExecution","id":"c2","command":"ls","status":"inProgress"}}`)
	notify("item/completed", `{"threadId":"t","item":{"type":"commandExecution","id":"c2","command":"ls","status":"completed","exitCode":0}}`)
	if got := background(); len(got) != 0 {
		t.Fatalf("parallel calls became background: %+v", got)
	}

	clock = clock.Add(2 * time.Second)
	notify("item/started", `{"threadId":"t","item":{"type":"agentMessage","id":"m1"}}`)
	got := background()
	if len(got) != 1 || got[0].ID != "c1" || got[0].Kind != "shell" || got[0].Status != "running" ||
		got[0].Description != "make test" || got[0].StartedAt != 1_000_000 {
		t.Fatalf("left running = %+v", got)
	}

	clock = clock.Add(3 * time.Second)
	notify("item/completed", `{"threadId":"t","item":{"type":"commandExecution","id":"c1","command":"make test","status":"completed","exitCode":2}}`)
	got = background()
	if len(got) != 1 || got[0].Status != "failed" || got[0].EndedAt != 1_005_000 || got[0].Summary != "exit code 2" {
		t.Fatalf("finished = %+v", got)
	}
}
