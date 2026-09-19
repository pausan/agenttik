package direct

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

func events(t *testing.T, client *Client, req agent.TurnRequest) []agent.Event {
	t.Helper()
	ch, err := client.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var out []agent.Event
	for e := range ch {
		out = append(out, e)
	}
	return out
}
func emitJSON(w http.ResponseWriter, value any) {
	b, _ := json.Marshal(value)
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func TestEnvironmentPromptAndPermissions(t *testing.T) {
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "AGENTS.md"), []byte("Use the test fixture convention."), 0600); err != nil {
		t.Fatal(err)
	}
	env := environment{Shells: []shellInfo{{Name: "bash", Path: "/bin/bash"}}, Python: "/usr/bin/python3"}
	req := agent.TurnRequest{WorkDir: work, Permission: agent.PermissionWorkspace}
	prompt := systemPrompt(req, env)
	for _, text := range []string{"Use the test fixture convention.", "/bin/bash", "/usr/bin/python3", "concise summary", "iterate", "not permitted"} {
		if !strings.Contains(prompt, text) {
			t.Errorf("prompt missing %q", text)
		}
	}
	for _, permission := range []agent.Permission{agent.PermissionPlan, agent.PermissionWorkspace, agent.PermissionFull} {
		tools := codingTools(permission, env)
		names := map[string]bool{}
		for _, tool := range tools {
			names[tool.(map[string]any)["function"].(map[string]any)["name"].(string)] = true
		}
		if names["shell"] != (permission == agent.PermissionFull) || names["python"] != (permission == agent.PermissionFull) || names["write_file"] != (permission != agent.PermissionPlan) {
			t.Fatalf("permission %s: %v", permission, names)
		}
	}
	if len(codingTools(agent.PermissionFull, environment{})) != 3 {
		t.Fatal("advertised missing executables")
	}
	req.Isolated = true
	req.Permission = agent.PermissionPlan
	if strings.Contains(systemPrompt(req, env), "fixture convention") {
		t.Fatal("metadata inherited repository instructions")
	}
}

func TestProbeAndRunPythonAndShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	for name, body := range map[string]string{"bash": "#!/bin/sh\nexec /bin/sh \"$@\"\n", "python3": "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'Python 3.13.0'; else echo python-ran; fi\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	env := detectEnvironment()
	if len(env.Shells) != 1 || env.Python != filepath.Join(dir, "python3") {
		t.Fatal(env)
	}
	req := agent.TurnRequest{WorkDir: dir, Permission: agent.PermissionFull}
	for _, test := range []struct{ name, args, want string }{{"shell", `{"command":"echo shell-ran","shell":"bash"}`, "shell-ran"}, {"python", `{"code":"print('python-ran')"}`, "python-ran"}} {
		call := ToolCall{}
		call.Function.Name, call.Function.Arguments = test.name, test.args
		got, err := runTool(context.Background(), req, call, env)
		if err != nil || strings.TrimSpace(got) != test.want {
			t.Fatalf("%s: %s, %v", test.name, got, err)
		}
		req.Permission = agent.PermissionWorkspace
		if _, err := runTool(context.Background(), req, call, env); err == nil {
			t.Fatal("workspace executed command")
		}
		req.Permission = agent.PermissionFull
	}
	t.Setenv("PATH", t.TempDir())
	if env := detectEnvironment(); len(env.Shells) != 0 || env.Python != "" {
		t.Fatal("invented executable", env)
	}
}

func TestAnthropicToolsAndResume(t *testing.T) {
	work, history := t.TempDir(), t.TempDir()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("x-api-key") != "secret" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
			t.Error("invalid Anthropic authentication")
		}
		var body struct {
			System   string `json:"system"`
			Messages []struct {
				Role    string           `json:"role"`
				Content []map[string]any `json:"content"`
			} `json:"messages"`
			Tools []any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !strings.Contains(body.System, "concise summary") || len(body.Tools) != 3 {
			t.Error("missing runtime prompt/tools")
		}
		emitJSON(w, map[string]any{"type": "message_start", "message": map[string]any{"usage": map[string]int{"input_tokens": 3, "cache_read_input_tokens": 2}}})
		if requests == 1 {
			emitJSON(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "tool-1", "name": "write_file", "input": map[string]any{}}})
			for _, part := range []string{`{"path":"nested/hello.txt",`, `"content":"hello"}`} {
				emitJSON(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": part}})
			}
			emitJSON(w, map[string]any{"type": "message_delta", "delta": map[string]string{"stop_reason": "tool_use"}, "usage": map[string]int{"output_tokens": 4}})
		} else {
			if requests == 2 {
				last := body.Messages[len(body.Messages)-1]
				if last.Role != "user" || last.Content[0]["type"] != "tool_result" || last.Content[0]["tool_use_id"] != "tool-1" {
					t.Error("missing native tool result", last)
				}
			}
			if requests == 3 && len(body.Messages) != 5 {
				t.Errorf("resume has %d messages", len(body.Messages))
			}
			emitJSON(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]string{"type": "text", "text": ""}})
			emitJSON(w, map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]string{"type": "text_delta", "text": "Summary: wrote hello."}})
			emitJSON(w, map[string]any{"type": "message_delta", "delta": map[string]string{"stop_reason": "end_turn"}, "usage": map[string]int{"output_tokens": 5}})
		}
		emitJSON(w, map[string]string{"type": "message_stop"})
	}))
	defer server.Close()
	client := &Client{HTTP: server.Client(), Endpoint: server.URL, Protocol: "anthropic", Key: "secret", HistoryDir: history}
	req := agent.TurnRequest{WorkDir: work, Model: "claude-test", Prompt: "write hello", Permission: agent.PermissionWorkspace}
	out := events(t, client, req)
	last := out[len(out)-1]
	if last.Type != agent.EventDone || last.Usage.InputTokens != 6 || last.Usage.OutputTokens != 9 || last.Usage.CacheReadTokens != 4 {
		t.Fatal(out)
	}
	data, err := os.ReadFile(filepath.Join(work, "nested", "hello.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatal("file not written", err)
	}
	req.ProviderSessionID = out[0].ProviderSessionID
	req.Prompt = "what happened?"
	out = events(t, client, req)
	if out[len(out)-1].Type != agent.EventDone {
		t.Fatal(out)
	}
	if requests != 3 {
		t.Fatal(requests)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(history, req.ProviderSessionID+".json"))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("history must be private", err)
		}
	}
}

func TestChatReasoningAndToolSignatures(t *testing.T) {
	input := `data: {"choices":[{"delta":{"reasoning":"thinking","reasoning_details":[{"type":"reasoning.text","index":0,"text":"part 1"}],"tool_calls":[{"index":0,"id":"t","function":{"name":"read_file","arguments":"{\"path\":\"AGENTS.md\"}"},"extra_content":{"google":{"thought_signature":"signed"}}}]}}]}

data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.text","index":0,"text":"part 2","signature":"verified"}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	reply, _, err := ReadCompletion(strings.NewReader(input), func(agent.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	wire := chatMessages([]Message{reply})[0]
	if wire["reasoning"] != "thinking" || wire["reasoning_content"] != nil || len(reply.ReasoningDetails) != 1 || reply.ReasoningDetails[0]["text"] != "part 1part 2" || !strings.Contains(string(reply.Calls[0].Extra), "signed") {
		t.Fatal(wire)
	}
}

func TestFailureKeepsCompletedToolHistory(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"t\",\"function\":{\"name\":\"write_file\",\"arguments\":\"{\\\"path\\\":\\\"done\\\",\\\"content\\\":\\\"done\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		} else {
			w.WriteHeader(429)
			fmt.Fprint(w, "never return this secret")
		}
	}))
	defer server.Close()
	history := t.TempDir()
	client := &Client{HTTP: server.Client(), Endpoint: server.URL, HistoryDir: history}
	out := events(t, client, agent.TurnRequest{WorkDir: t.TempDir(), Permission: agent.PermissionWorkspace, Prompt: "do it"})
	if out[len(out)-1].Type != agent.EventError || strings.Contains(out[len(out)-1].Text, "secret") {
		t.Fatal(out)
	}
	var saved conversation
	b, err := os.ReadFile(filepath.Join(history, out[0].ProviderSessionID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Messages) != 3 || saved.Messages[2].Role != "tool" {
		t.Fatal("lost completed tools", saved)
	}
}

func TestMalformedStreamsAndCancellation(t *testing.T) {
	for _, data := range []string{`data: {"type":"error"}`, `data: {"type":"message_stop"}`, `data: {"type":"content_block_delta","index":3}`, `data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}` + "\n\ndata: {\"type\":\"message_stop\"}"} {
		if _, _, err := readAnthropic(strings.NewReader(data), func(agent.Event) {}); err == nil {
			t.Error("accepted broken stream", data)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	env := detectEnvironment()
	if len(env.Shells) == 0 {
		t.Skip("no shell")
	}
	call := ToolCall{}
	call.Function.Name = "shell"
	call.Function.Arguments = `{"command":"sleep 30"}`
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := runTool(ctx, agent.TurnRequest{WorkDir: t.TempDir(), Permission: agent.PermissionFull}, call, env); err == nil {
		t.Fatal("cancelled command succeeded")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("command did not cancel")
	}
}
