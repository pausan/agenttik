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
		"--permission-mode auto", "--session-id abc",
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
		agent.PermissionWorkspace: "--permission-mode auto",
		agent.PermissionFull:      "--dangerously-skip-permissions",
		agent.Permission("junk"):  "--permission-mode auto",
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
	parse(strings.NewReader(stream), "opus", out)
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

// The context gauge needs the size of one prompt, so an assistant message's
// own usage is reported separately from the turn's totals in the result.
func TestParseAssistantUsageIsContextOnly(t *testing.T) {
	line := `{"type":"assistant","message":{"usage":{"input_tokens":12,"output_tokens":40,` +
		`"cache_read_input_tokens":30000,"cache_creation_input_tokens":500},"content":[]}}`
	got := collect(t, line+"\n")
	if len(got) != 1 || got[0].Type != agent.EventUsage {
		t.Fatalf("got %+v", got)
	}
	if n := got[0].Usage.ContextTokens; n != 30512 {
		t.Errorf("context tokens = %d, want 30512", n)
	}
	if got[0].Usage.OutputTokens != 0 {
		t.Errorf("mid-turn usage must not carry turn totals: %+v", got[0].Usage)
	}
}

func TestParseAssistantWithoutUsageEmitsNothing(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`
	if got := collect(t, line+"\n"); len(got) != 0 {
		t.Errorf("got %+v, want no events", got)
	}
}

func TestParseSeparatesSubagentUsageFromMainContext(t *testing.T) {
	stream := `{"type":"assistant","message":{"usage":{"input_tokens":10,"output_tokens":2,` +
		`"cache_read_input_tokens":100,"cache_creation_input_tokens":5},"content":[]}}` + "\n" +
		`{"type":"assistant","parent_tool_use_id":"agent-1","message":{"usage":{` +
		`"input_tokens":20,"output_tokens":4,"cache_read_input_tokens":200,` +
		`"cache_creation_input_tokens":6},"content":[]}}` + "\n" +
		`{"type":"assistant","parent_tool_use_id":"agent-1","message":{"usage":{` +
		`"input_tokens":30,"output_tokens":5,"cache_read_input_tokens":300,` +
		`"cache_creation_input_tokens":7},"content":[]}}` + "\n" +
		`{"type":"result","usage":{"input_tokens":60,"output_tokens":11,` +
		`"cache_read_input_tokens":600,"cache_creation_input_tokens":18}}` + "\n"

	got := collect(t, stream)
	if len(got) != 2 || got[0].Type != agent.EventUsage || got[1].Type != agent.EventDone {
		t.Fatalf("got %+v", got)
	}
	if got[0].Usage.ContextTokens != 115 {
		t.Errorf("main context = %d, want 115", got[0].Usage.ContextTokens)
	}
	u := got[1].Usage
	if u.ContextTokens != 115 || u.MainInputTokens != 10 || u.MainOutputTokens != 2 ||
		u.MainCacheReadTokens != 100 || u.MainCacheWriteTokens != 5 ||
		u.SubagentInputTokens != 50 || u.SubagentOutputTokens != 9 ||
		u.SubagentCacheReadTokens != 500 || u.SubagentCacheWriteTokens != 13 ||
		u.SubagentCount != 1 || !u.UsageBreakdown {
		t.Errorf("usage = %+v", u)
	}
}
