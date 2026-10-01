package claudecode

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
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

func TestBuildEnvWaitsForMCPServers(t *testing.T) {
	t.Setenv("MCP_CONNECTION_NONBLOCKING", "")
	os.Unsetenv("MCP_CONNECTION_NONBLOCKING")
	if !slices.Contains(buildEnv(agent.TurnRequest{}), "MCP_CONNECTION_NONBLOCKING=0") {
		t.Error("turns should wait for MCP servers")
	}
	if env := buildEnv(agent.TurnRequest{AccountHome: "/x"}); !slices.Contains(env, "MCP_CONNECTION_NONBLOCKING=0") ||
		!slices.Contains(env, HomeVar+"=/x") {
		t.Errorf("account turn env missing home or MCP wait: %v", env)
	}
	if env := buildEnv(agent.TurnRequest{Isolated: true}); env != nil {
		t.Errorf("metadata requests should inherit the env untouched: %v", env)
	}
	t.Setenv("MCP_CONNECTION_NONBLOCKING", "1")
	if env := buildEnv(agent.TurnRequest{}); env != nil {
		t.Errorf("a user-set value should win: %v", env)
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
	parse(strings.NewReader(stream), "opus", nil, out)
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

func TestIsolatedConflictResolutionAllowsTools(t *testing.T) {
	for _, permission := range []agent.Permission{agent.PermissionPlan, agent.PermissionWorkspace} {
		args := buildArgs(agent.TurnRequest{Isolated: true, Permission: permission})
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--no-session-persistence") {
			t.Fatal(args)
		}
		hasToolsFlag := false
		for _, arg := range args {
			if arg == "--tools" {
				hasToolsFlag = true
			}
		}
		if hasToolsFlag != (permission == agent.PermissionPlan) {
			t.Fatal(args)
		}
	}
}

// pipe is a stdin stand-in that records what the host writes.
type pipe struct {
	bytes.Buffer
	closed bool
}

func (p *pipe) Close() error { p.closed = true; return nil }

func (p *pipe) lines(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(p.String()), "\n") {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("host wrote %q: %v", line, err)
		}
		out = append(out, v)
	}
	return out
}

const approvalStream = `{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"mcp__claude_ai_Linear__create_issue","display_name":"Linear: create_issue","description":"Create an issue","input":{"title":"Bug"}}}
{"type":"control_request","request_id":"r2","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[]}}}
{"type":"control_request","request_id":"r3","request":{"subtype":"hook_callback"}}
{"type":"control_cancel_request","request_id":"r4"}
{"type":"result","subtype":"success","usage":{}}
`

func TestParseTurnsToolRequestsIntoApprovals(t *testing.T) {
	stdin := &pipe{}
	h := &host{w: stdin}
	out := make(chan agent.Event, 16)
	parse(strings.NewReader(approvalStream), "opus", h, out)
	close(out)
	var approvals, resolved []*agent.Approval
	for ev := range out {
		switch ev.Type {
		case agent.EventApproval:
			approvals = append(approvals, ev.Approval)
		case agent.EventApprovalResolved:
			resolved = append(resolved, ev.Approval)
		}
	}
	if len(approvals) != 1 || approvals[0].ID != "r1" || approvals[0].Tool != "Linear: create_issue" ||
		approvals[0].Input != `{"title":"Bug"}` {
		t.Fatalf("approvals = %+v", approvals)
	}
	if len(resolved) != 1 || resolved[0].ID != "r4" {
		t.Fatalf("resolved = %+v", resolved)
	}
	// The question tool and the unknown request are answered at once, and the
	// result closes stdin.
	lines := stdin.lines(t)
	if len(lines) != 2 || !stdin.closed {
		t.Fatalf("host wrote %v, closed=%v", lines, stdin.closed)
	}
	if r := lines[0]["response"].(map[string]any); r["request_id"] != "r2" ||
		r["response"].(map[string]any)["behavior"] != "deny" {
		t.Errorf("AskUserQuestion answer = %v", lines[0])
	}
	if r := lines[1]["response"].(map[string]any); r["request_id"] != "r3" || r["subtype"] != "error" {
		t.Errorf("unknown request answer = %v", lines[1])
	}
	// Answering after the turn ended reports it instead of writing.
	if err := approvals[0].Answer(true); err == nil {
		t.Error("answer after close should fail")
	}
}

func TestApprovalAnswerWritesControlResponse(t *testing.T) {
	for _, allow := range []bool{true, false} {
		stdin := &pipe{}
		h := &host{w: stdin}
		out := make(chan agent.Event, 4)
		p := streamParser{host: h, subagents: map[string]struct{}{}}
		p.handleLine([]byte(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"}}}`), out)
		ev := <-out
		if err := ev.Approval.Answer(allow); err != nil {
			t.Fatal(err)
		}
		resp := stdin.lines(t)[0]["response"].(map[string]any)
		result := resp["response"].(map[string]any)
		if resp["request_id"] != "r1" || resp["subtype"] != "success" {
			t.Errorf("response = %v", resp)
		}
		if allow && (result["behavior"] != "allow" || result["updatedInput"].(map[string]any)["command"] != "ls") {
			t.Errorf("allow result = %v", result)
		}
		if !allow && (result["behavior"] != "deny" || result["message"] == "") {
			t.Errorf("deny result = %v", result)
		}
	}
}

func TestPromptIsAStreamJSONLine(t *testing.T) {
	stdin := &pipe{}
	if err := (&host{w: stdin}).sendPrompt("hi\nthere"); err != nil {
		t.Fatal(err)
	}
	line := stdin.lines(t)[0]
	if line["type"] != "user" || line["message"].(map[string]any)["content"] != "hi\nthere" {
		t.Errorf("prompt line = %v", line)
	}
}

func TestBuildArgsAsksOnlyForTaskTurns(t *testing.T) {
	task := strings.Join(buildArgs(agent.TurnRequest{}), " ")
	if !strings.Contains(task, "--input-format stream-json --permission-prompt-tool stdio") {
		t.Errorf("task turn args %q should prompt over stdio", task)
	}
	isolated := strings.Join(buildArgs(agent.TurnRequest{Isolated: true}), " ")
	if strings.Contains(isolated, "--permission-prompt-tool") || strings.Contains(isolated, "--input-format") {
		t.Errorf("isolated args %q must not prompt", isolated)
	}
}
