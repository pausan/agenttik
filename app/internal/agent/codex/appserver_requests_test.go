package codex

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

// requestRun is an app-server run whose writes land in a buffer.
func requestRun() (*appServerRun, *bytes.Buffer, chan agent.Event) {
	var buf bytes.Buffer
	out := make(chan agent.Event, 8)
	return &appServerRun{out: out, enc: json.NewEncoder(&buf),
		pending: make(map[string]string), fileChanges: make(map[string][]string),
		streamed: make(map[string]bool), activeTurns: make(map[string]string)}, &buf, out
}

func serverRequest(t *testing.T, run *appServerRun, id, method, params string) {
	t.Helper()
	if _, err := run.handle(appRPCMessage{ID: json.RawMessage(id), Method: method, Params: json.RawMessage(params)}); err != nil {
		t.Fatal(err)
	}
}

// written decodes the last message the run wrote.
func written(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var v map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &v); err != nil {
		t.Fatalf("wrote %q: %v", buf.String(), err)
	}
	return v
}

func TestCommandApprovalWaitsForTheUser(t *testing.T) {
	for allow, want := range map[bool]string{true: "accept", false: "decline"} {
		run, buf, out := requestRun()
		serverRequest(t, run, "7", "item/commandExecution/requestApproval",
			`{"command":"rm -rf build","cwd":"/p","reason":"Outside the workspace"}`)
		if buf.Len() != 0 {
			t.Fatalf("answered before the user did: %s", buf)
		}
		a := (<-out).Approval
		if a.Tool != "Shell" || a.Description != "Outside the workspace" || !strings.Contains(a.Input, `"command":"rm -rf build"`) {
			t.Fatalf("approval = %+v", a)
		}
		if err := a.Answer(agent.Reply{Allow: allow}); err != nil {
			t.Fatal(err)
		}
		if got := written(t, buf); got["id"] != float64(7) || got["result"].(map[string]any)["decision"] != want {
			t.Errorf("allow=%v wrote %v", allow, got)
		}
	}
}

func TestFileChangeApprovalNamesTheFiles(t *testing.T) {
	run, buf, out := requestRun()
	run.rootID = "root"
	run.handleNotification("item/started", json.RawMessage(
		`{"threadId":"root","item":{"type":"fileChange","id":"fc1","changes":[{"path":"a.go","kind":"update","diff":""}]}}`))
	serverRequest(t, run, `"x"`, "item/fileChange/requestApproval", `{"itemId":"fc1"}`)
	var a *agent.Approval
	for ev := range out {
		if ev.Type == agent.EventApproval {
			a = ev.Approval
			break
		}
	}
	if a.Tool != "Edit files" || a.Input != `{"files":["a.go"]}` {
		t.Fatalf("approval = %+v", a)
	}
	a.Answer(agent.Reply{Allow: true})
	if got := written(t, buf); got["id"] != "x" || got["result"].(map[string]any)["decision"] != "accept" {
		t.Errorf("wrote %v", got)
	}
}

func TestUserInputBecomesQuestions(t *testing.T) {
	run, buf, out := requestRun()
	serverRequest(t, run, "3", "item/tool/requestUserInput",
		`{"questions":[{"id":"color","header":"Color","question":"Which color?","isOther":true,`+
			`"options":[{"label":"Red","description":"warm"},{"label":"Blue","description":""}]},`+
			`{"id":"name","header":"Name","question":"Your name?","isSecret":true}]}`)
	a := (<-out).Approval
	if len(a.Questions) != 2 || a.Questions[0].ID != "color" || !a.Questions[0].Other ||
		len(a.Questions[0].Options) != 2 || !a.Questions[1].Other || !a.Questions[1].Secret {
		t.Fatalf("questions = %+v", a.Questions)
	}
	a.Answer(agent.Reply{Allow: true, Answers: map[string][]string{"color": {"Blue"}, "name": {"Ada"}}})
	answers := written(t, buf)["result"].(map[string]any)["answers"].(map[string]any)
	if answers["color"].(map[string]any)["answers"].([]any)[0] != "Blue" ||
		answers["name"].(map[string]any)["answers"].([]any)[0] != "Ada" {
		t.Errorf("answers = %v", answers)
	}
}

func TestMCPToolElicitationIsAnApproval(t *testing.T) {
	run, buf, out := requestRun()
	serverRequest(t, run, "4", "mcpServer/elicitation/request",
		`{"serverName":"tracker","mode":"form","message":"Allow the tracker MCP server to run tool \"create_issue\"?",`+
			`"requestedSchema":{"type":"object","properties":{}},`+
			`"_meta":{"codex_approval_kind":"mcp_tool_call","tool_params":{"title":"Bug"}}}`)
	a := (<-out).Approval
	if a.Tool != "tracker MCP tool" || a.Input != `{"title":"Bug"}` || len(a.Questions) != 0 {
		t.Fatalf("approval = %+v", a)
	}
	a.Answer(agent.Reply{Allow: false})
	if got := written(t, buf)["result"].(map[string]any); got["action"] != "decline" {
		t.Errorf("declined wrote %v", got)
	}
}

func TestMCPFormElicitationTypesItsAnswers(t *testing.T) {
	run, buf, out := requestRun()
	serverRequest(t, run, "5", "mcpServer/elicitation/request",
		`{"serverName":"deploy","mode":"form","message":"Deploy where?","requestedSchema":{"type":"object","properties":{`+
			`"env":{"type":"string","oneOf":[{"const":"prod","title":"Production"},{"const":"stg","title":"Staging"}]},`+
			`"replicas":{"type":"integer","title":"Replicas"},`+
			`"notify":{"type":"boolean"},`+
			`"regions":{"type":"array","items":{"type":"string","enum":["eu","us"]}}}}}`)
	a := (<-out).Approval
	if len(a.Questions) != 4 || a.Questions[0].ID != "env" || a.Questions[0].Options[0].Label != "Production" ||
		!a.Questions[3].Multi || !a.Questions[1].Other || a.Questions[1].Question != "Replicas" {
		t.Fatalf("questions = %+v", a.Questions)
	}
	reply := agent.Reply{Allow: true, Answers: map[string][]string{
		"env": {"Staging"}, "notify": {"Yes"}, "regions": {"eu", "us"}, "replicas": {"3"}}}
	if err := a.CheckReply(reply); err != nil {
		t.Fatal(err)
	}
	a.Answer(reply)
	content := written(t, buf)["result"].(map[string]any)["content"].(map[string]any)
	if content["env"] != "stg" || content["notify"] != true || content["replicas"] != float64(3) ||
		len(content["regions"].([]any)) != 2 {
		t.Errorf("content = %v", content)
	}
}

func TestURLElicitationAndUnknownRequestsAreRefusedAtOnce(t *testing.T) {
	run, buf, out := requestRun()
	serverRequest(t, run, "6", "mcpServer/elicitation/request",
		`{"serverName":"s","mode":"url","message":"Sign in","url":"https://x","elicitationId":"e"}`)
	if got := written(t, buf)["result"].(map[string]any); got["action"] != "decline" {
		t.Errorf("url elicitation wrote %v", got)
	}
	serverRequest(t, run, "8", "item/permissions/requestApproval", `{}`)
	if got := written(t, buf); got["error"] == nil {
		t.Errorf("unknown request wrote %v", got)
	}
	if len(out) != 0 {
		t.Errorf("nothing should be asked, got %d events", len(out))
	}
}

func TestResolvedRequestIsWithdrawnOnceAndLateAnswersFail(t *testing.T) {
	run, _, out := requestRun()
	serverRequest(t, run, "9", "item/commandExecution/requestApproval", `{"command":"ls"}`)
	a := (<-out).Approval
	run.handleNotification("serverRequest/resolved", json.RawMessage(`{"threadId":"t","requestId":9}`))
	run.handleNotification("serverRequest/resolved", json.RawMessage(`{"threadId":"t","requestId":9}`))
	if ev := <-out; ev.Type != agent.EventApprovalResolved || ev.Approval.ID != a.ID {
		t.Fatalf("resolved = %+v", ev)
	}
	if len(out) != 0 {
		t.Error("a request is withdrawn once")
	}
	run.closeWrites()
	if err := a.Answer(agent.Reply{Allow: true}); err == nil {
		t.Error("answer after the turn should fail")
	}
}
