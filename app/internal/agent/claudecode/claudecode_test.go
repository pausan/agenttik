package claudecode

import (
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestBuildArgsFirstTurnPicksSessionID(t *testing.T) {
	args := buildArgs(agent.TurnRequest{
		SessionID: "abc", Model: "opus", Effort: "high",
		Permission: agent.PermissionWorkspace})
	got := strings.Join(args, " ")
	for _, want := range []string{
		"--output-format stream-json", "--include-partial-messages",
		"--model opus", "--effort high",
		"--permission-mode acceptEdits", "--session-id abc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "--resume") {
		t.Errorf("first turn must not resume: %q", got)
	}
}

func TestBuildArgsResumesWithProviderSessionID(t *testing.T) {
	args := buildArgs(agent.TurnRequest{SessionID: "abc", ProviderSessionID: "xyz"})
	got := strings.Join(args, " ")
	if !strings.Contains(got, "--resume xyz") {
		t.Errorf("args %q should resume xyz", got)
	}
	if strings.Contains(got, "--session-id") {
		t.Errorf("resume must not also set --session-id: %q", got)
	}
}

func TestPermissionMapping(t *testing.T) {
	cases := map[agent.Permission]string{
		agent.PermissionPlan:      "--permission-mode plan",
		agent.PermissionWorkspace: "--permission-mode acceptEdits",
		agent.PermissionFull:      "--dangerously-skip-permissions",
		agent.Permission("junk"):  "--permission-mode acceptEdits",
	}
	for perm, want := range cases {
		got := strings.Join(permissionFlag(perm), " ")
		if got != want {
			t.Errorf("permission %q -> %q, want %q", perm, got, want)
		}
	}
}

// A representative stream, in the order the CLI emits it.
const sampleStream = `{"type":"system","subtype":"init","session_id":"sess-1","model":"opus"}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hmm"}}}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"file body"}]}}
not json at all
{"type":"result","subtype":"success","total_cost_usd":0.25,"usage":{"input_tokens":10,"output_tokens":3,"cache_read_input_tokens":7,"cache_creation_input_tokens":2}}
`

func collect(t *testing.T, stream string) []agent.Event {
	t.Helper()
	out := make(chan agent.Event, 64)
	parse(strings.NewReader(stream), out)
	close(out)
	var got []agent.Event
	for ev := range out {
		got = append(got, ev)
	}
	return got
}

func TestParseStream(t *testing.T) {
	got := collect(t, sampleStream)
	want := []agent.EventType{
		agent.EventSessionStarted, agent.EventThinking,
		agent.EventText, agent.EventText,
		agent.EventToolUse, agent.EventToolResult, agent.EventDone,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w {
			t.Errorf("event %d: got %q, want %q", i, got[i].Type, w)
		}
	}
	if got[0].ProviderSessionID != "sess-1" {
		t.Errorf("session id = %q", got[0].ProviderSessionID)
	}
	if got[2].Text+got[3].Text != "Hello world" {
		t.Errorf("text = %q", got[2].Text+got[3].Text)
	}
	if got[4].Tool.Name != "Read" {
		t.Errorf("tool name = %q", got[4].Tool.Name)
	}
	if got[5].Tool.Output != "file body" {
		t.Errorf("tool output = %q", got[5].Tool.Output)
	}
	u := got[6].Usage
	if u == nil || u.InputTokens != 10 || u.OutputTokens != 3 ||
		u.CacheReadTokens != 7 || u.CacheWriteTokens != 2 || u.CostUSD != 0.25 {
		t.Errorf("usage = %+v", u)
	}
}

func TestParseToolResultBlocks(t *testing.T) {
	line := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}]}}`
	got := collect(t, line+"\n")
	if len(got) != 1 || got[0].Tool.Output != "ab" {
		t.Errorf("got %+v", got)
	}
}

func TestParseResultError(t *testing.T) {
	line := `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`
	got := collect(t, line+"\n")
	if len(got) != 2 || got[0].Type != agent.EventError || got[0].Text != "boom" {
		t.Fatalf("got %+v", got)
	}
	if got[1].Type != agent.EventDone {
		t.Errorf("error result must still finish the turn, got %q", got[1].Type)
	}
}

func TestParseHandlesVeryLongLine(t *testing.T) {
	big := strings.Repeat("x", 300_000)
	line := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"` + big + `"}]}}`
	got := collect(t, line+"\n")
	if len(got) != 1 || len(got[0].Tool.Output) != len(big) {
		t.Fatalf("long line truncated: got %d events", len(got))
	}
}
