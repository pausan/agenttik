package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/direct"
)

func TestAccountIsolationAndPrivateKey(t *testing.T) {
	p := New()
	system, work := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", system)
	if err := p.ConfigureAccount("", "auto", "system-key"); err != nil {
		t.Fatal(err)
	}
	if p.AccountStatus(work).SignedIn {
		t.Fatal("blank named account inherited system key")
	}
	if err := p.ConfigureAccount(work, "direct", "work-key"); err != nil {
		t.Fatal(err)
	}
	if p.key("") != "system-key" || p.key(work) != "work-key" {
		t.Fatal("accounts crossed")
	}
	if err := p.ConfigureAccount(work, "cli", ""); err != nil {
		t.Fatal(err)
	}
	if p.key(work) != "work-key" || p.ConnectionMode(work) != "cli" {
		t.Fatal("mode change lost key")
	}
	if err := p.ConfigureAccount(work, "bad", "replace"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	if p.key(work) != "work-key" {
		t.Fatal("invalid save changed key")
	}
	info, err := os.Stat(p.authPath(work))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode())
	}
	// Saving Go must preserve other providers in the CLI's shared auth file.
	b, _ := os.ReadFile(p.authPath(work))
	var auth map[string]json.RawMessage
	_ = json.Unmarshal(b, &auth)
	auth["other"] = json.RawMessage(`{"key":"other-key"}`)
	b, _ = json.Marshal(auth)
	_ = os.WriteFile(p.authPath(work), b, 0600)
	if err := p.ConfigureAccount(work, "direct", "new-key"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p.authPath(work))
	if !strings.Contains(string(b), "other-key") {
		t.Fatal("other provider removed")
	}
}
func TestAutomaticCLIAndExplicitDirect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := New()
	req := agent.TurnRequest{}
	if p.useCLI(req) {
		t.Fatal("missing CLI selected")
	}
	if err := os.WriteFile(filepath.Join(dir, Binary), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if !p.useCLI(req) {
		t.Fatal("installed CLI not default")
	}
	if err := p.ConfigureAccount("", "direct", "key"); err != nil {
		t.Fatal(err)
	}
	if p.useCLI(req) {
		t.Fatal("direct ignored")
	}
	if err := p.ConfigureAccount("", "auto", ""); err != nil {
		t.Fatal(err)
	}
	req.ProviderSessionID = "direct-existing"
	if p.useCLI(req) {
		t.Fatal("auto changed existing conversation backend")
	}
}
func stream(w http.ResponseWriter, value any) {
	b, _ := json.Marshal(value)
	fmt.Fprintf(w, "data: %s\n\n", b)
}
func completion(w http.ResponseWriter, text string) {
	stream(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": text}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 3, "prompt_tokens_details": map[string]int{"cached_tokens": 4}}})
	fmt.Fprint(w, "data: [DONE]\n\n")
}
func collect(t *testing.T, ch <-chan agent.Event) []agent.Event {
	t.Helper()
	var events []agent.Event
	for e := range ch {
		if e.Type == agent.EventError {
			t.Fatal(e.Text)
		}
		events = append(events, e)
	}
	return events
}
func TestDirectCodingTurnResumeAndHeaders(t *testing.T) {
	p := New()
	home, work := t.TempDir(), t.TempDir()
	if err := p.ConfigureAccount(home, "direct", "test-key"); err != nil {
		t.Fatal(err)
	}
	count := 0
	session := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("User-Agent") != "agenttik/1.0" {
			t.Error("missing auth or own user agent")
		}
		if session == "" {
			session = r.Header.Get("x-opencode-session")
		}
		if session == "" || session != r.Header.Get("x-opencode-session") {
			t.Error("unstable session")
		}
		var body struct {
			Messages []direct.Message `json:"messages"`
			Tools    []any            `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("bad body")
		}
		if len(body.Tools) == 0 {
			t.Error("no coding tools")
		}
		if count == 1 {
			stream(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_1", "function": map[string]string{"name": "write_file", "arguments": `{"path":"hello.txt","content":"hello"}`}}}}, "finish_reason": "tool_calls"}}})
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		if count == 2 && body.Messages[len(body.Messages)-1].Role != "tool" {
			t.Error("tool result missing")
		}
		if count == 3 && len(body.Messages) < 6 {
			t.Error("resume lost history")
		}
		completion(w, "Done")
	}))
	defer server.Close()
	p.endpoint = server.URL
	req := agent.TurnRequest{AccountHome: home, WorkDir: work, Prompt: "write hello", Permission: agent.PermissionWorkspace}
	ch, err := p.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	b, err := os.ReadFile(filepath.Join(work, "hello.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatal("tool did not write file", err)
	}
	req.ProviderSessionID = events[0].ProviderSessionID
	req.Prompt = "what did you do?"
	ch, err = p.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	events = collect(t, ch)
	last := events[len(events)-1]
	if last.Type != agent.EventDone || last.Usage.InputTokens != 8 || last.Usage.CacheReadTokens != 4 {
		t.Fatalf("bad usage %+v", last)
	}
	if count != 3 {
		t.Fatalf("requests %d", count)
	}
	req.AccountHome = t.TempDir()
	if _, err = p.Run(context.Background(), req); err == nil {
		t.Fatal("unsigned account resumed another account")
	}
}
func TestPermissionAndPathBoundaries(t *testing.T) {
	work, outside := t.TempDir(), t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0600)
	req := agent.TurnRequest{WorkDir: work, Permission: agent.PermissionPlan}
	call := direct.ToolCall{}
	call.Function.Name = "write_file"
	call.Function.Arguments = `{"path":"x","content":"oops"}`
	if _, err := direct.RunTool(context.Background(), req, call); err == nil {
		t.Fatal("plan wrote file")
	}
	call.Function.Name = "shell"
	call.Function.Arguments = `{"command":"echo unsafe"}`
	if _, err := direct.RunTool(context.Background(), req, call); err == nil {
		t.Fatal("plan ran shell")
	}
	req.Permission = agent.PermissionWorkspace
	if _, err := direct.RunTool(context.Background(), req, call); err == nil {
		t.Fatal("workspace ran shell")
	}
	if err := os.Symlink(outside, filepath.Join(work, "escape")); err != nil {
		t.Skip(err)
	}
	call.Function.Name = "read_file"
	call.Function.Arguments = `{"path":"escape/secret"}`
	if _, err := direct.RunTool(context.Background(), req, call); err == nil {
		t.Fatal("symlink escaped project")
	}
	call.Function.Name = "write_file"
	call.Function.Arguments = `{"path":"escape/secret","content":"oops"}`
	if _, err := direct.RunTool(context.Background(), req, call); err == nil {
		t.Fatal("write escaped project")
	}
}
func TestDirectErrorsAndCancellation(t *testing.T) {
	for _, input := range []string{`data: {"error":{"message":"secret"}}`, "data: [DONE]\n", `data: {"choices":[{"delta":{},"finish_reason":"length"}]}`} {
		if _, _, err := direct.ReadCompletion(strings.NewReader(input), func(agent.Event) {}); err == nil {
			t.Errorf("accepted incomplete/error stream %q", input)
		}
	}
	p := New()
	home := t.TempDir()
	_ = p.ConfigureAccount(home, "direct", "key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer server.Close()
	p.endpoint = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	ch, err := p.Run(ctx, agent.TurnRequest{AccountHome: home, Isolated: true, Prompt: "test"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-time.After(time.Second):
		t.Fatal("did not cancel")
	case <-func() chan struct{} {
		done := make(chan struct{})
		go func() {
			for range ch {
			}
			close(done)
		}()
		return done
	}():
	}
}
func TestCLIStreamAndAccountEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	home, work, bin := t.TempDir(), t.TempDir(), t.TempDir()
	p := New()
	_ = p.ConfigureAccount(home, "auto", "key")
	t.Setenv("PATH", bin)
	script := `#!/bin/sh
if [ "$XDG_DATA_HOME" != "` + home + `" ]; then exit 2; fi
read -r prompt
if [ "$prompt" != "hello" ]; then exit 3; fi
echo '{"type":"text","sessionID":"ses_test","part":{"text":"hello"}}'
echo '{"type":"step_finish","sessionID":"ses_test","part":{"reason":"stop","tokens":{"input":2,"output":3,"cache":{"read":4,"write":1}}}}'
`
	_ = os.WriteFile(filepath.Join(bin, Binary), []byte(script), 0700)
	ch, err := p.Run(context.Background(), agent.TurnRequest{AccountHome: home, WorkDir: work, Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	if events[0].ProviderSessionID != "ses_test" || events[len(events)-1].Usage.CacheReadTokens != 4 {
		t.Fatalf("bad CLI events %+v", events)
	}
}

func TestFragmentedToolCall(t *testing.T) {
	var response strings.Builder
	for _, args := range []string{`{"pa`, `th":"hello.txt"}`} {
		chunk := map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]string{"arguments": args}}}}}}}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(&response, "data: %s\n\n", b)
	}
	response.WriteString("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
	reply, _, err := direct.ReadCompletion(strings.NewReader(response.String()), func(agent.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Calls) != 1 || reply.Calls[0].Function.Arguments != `{"path":"hello.txt"}` {
		t.Fatalf("fragmented call %+v", reply.Calls)
	}
}

func TestIsolatedConflictResolutionPermissions(t *testing.T) {
	for _, permission := range []agent.Permission{agent.PermissionPlan, agent.PermissionWorkspace} {
		var got map[string]string
		if err := json.Unmarshal([]byte(cliPermissions(agent.TurnRequest{Isolated: true, Permission: permission})), &got); err != nil {
			t.Fatal(err)
		}
		if (got["edit"] == "allow") != (permission == agent.PermissionWorkspace) {
			t.Fatal(got)
		}
	}
}
