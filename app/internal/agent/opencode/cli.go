package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/process"
)

func cliArgs(req agent.TurnRequest) []string {
	args := []string{"run", "--format", "json", "--model", "opencode-go/" + req.Model}
	if !req.Isolated && strings.HasPrefix(req.ProviderSessionID, "ses_") {
		args = append(args, "--session", req.ProviderSessionID)
	}
	return args
}
func cliPermissions(req agent.TurnRequest) string {
	permissions := map[string]string{"*": "deny", "read": "allow", "glob": "allow", "grep": "allow", "list": "allow"}
	if req.Permission.Valid() != agent.PermissionPlan {
		permissions["edit"] = "allow"
	}
	if req.Permission.Valid() == agent.PermissionFull {
		permissions = map[string]string{"*": "allow", "question": "deny"}
	}
	if req.Isolated {
		permissions = map[string]string{"*": "deny"}
	}
	b, _ := json.Marshal(permissions)
	return string(b)
}
func (p *Provider) runCLI(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	cmd := exec.CommandContext(ctx, Binary, cliArgs(req)...)
	cmd.Dir = req.WorkDir
	cmd.Stdin = strings.NewReader(req.Prompt)
	cmd.Env = append(os.Environ(), "OPENCODE_PERMISSION="+cliPermissions(req), "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_AUTO_SHARE=false")
	config, _ := json.Marshal(map[string]any{"provider": map[string]any{"opencode-go": map[string]any{"options": map[string]string{"apiKey": p.key(req.AccountHome), "baseURL": "https://opencode.ai/zen/go/v1"}}}})
	cmd.Env = append(cmd.Env, "OPENCODE_CONFIG_CONTENT="+string(config))
	if req.AccountHome != "" {
		cmd.Env = append(cmd.Env, "XDG_DATA_HOME="+req.AccountHome)
	}
	process.Configure(cmd)
	cmd.Cancel = func() error { return process.Kill(cmd.Process) }
	cmd.WaitDelay = time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &limitedOutput{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start OpenCode CLI: %w", err)
	}
	out := make(chan agent.Event, 64)
	go func() {
		defer close(out)
		emit := func(e agent.Event) {
			select {
			case out <- e:
			case <-ctx.Done():
			}
		}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		started, failed, finished := false, false, false
		total := agent.Usage{}
		for scanner.Scan() {
			var event struct {
				Type      string `json:"type"`
				SessionID string `json:"sessionID"`
				Part      struct {
					Text   string  `json:"text"`
					CallID string  `json:"callID"`
					Tool   string  `json:"tool"`
					Cost   float64 `json:"cost"`
					Reason string  `json:"reason"`
					State  struct {
						Status string          `json:"status"`
						Input  json.RawMessage `json:"input"`
						Output string          `json:"output"`
						Error  string          `json:"error"`
					} `json:"state"`
					Tokens struct {
						Input     int64 `json:"input"`
						Output    int64 `json:"output"`
						Reasoning int64 `json:"reasoning"`
						Cache     struct {
							Read  int64 `json:"read"`
							Write int64 `json:"write"`
						} `json:"cache"`
					} `json:"tokens"`
				} `json:"part"`
			}
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				failed = true
				emit(agent.Event{Type: agent.EventError, Text: "Invalid OpenCode CLI event"})
				break
			}
			if !started && event.SessionID != "" {
				started = true
				emit(agent.Event{Type: agent.EventSessionStarted, ProviderSessionID: event.SessionID})
			}
			part := event.Part
			switch event.Type {
			case "text":
				emit(agent.Event{Type: agent.EventText, Text: part.Text})
			case "reasoning":
				emit(agent.Event{Type: agent.EventThinking, Text: part.Text})
			case "tool_use":
				emit(agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{ID: part.CallID, Name: part.Tool, Input: string(part.State.Input)}})
				output := part.State.Output
				if part.State.Status == "error" {
					output = part.State.Error
				}
				emit(agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{ID: part.CallID, Name: part.Tool, Output: output, IsError: part.State.Status == "error"}})
			case "step_finish":
				total.InputTokens += part.Tokens.Input
				total.OutputTokens += part.Tokens.Output + part.Tokens.Reasoning
				total.CacheReadTokens += part.Tokens.Cache.Read
				total.CacheWriteTokens += part.Tokens.Cache.Write
				total.CostUSD += part.Cost
				total.ContextTokens = part.Tokens.Input + part.Tokens.Cache.Read + part.Tokens.Cache.Write
				finished = part.Reason == "stop"
			case "error":
				failed = true
				emit(agent.Event{Type: agent.EventError, Text: "OpenCode CLI reported an error; check the subscription login and allowance"})
			}
		}
		if err := scanner.Err(); err != nil {
			failed = true
			emit(agent.Event{Type: agent.EventError, Text: err.Error()})
		}
		if failed {
			_ = process.Kill(cmd.Process)
		}
		if err := cmd.Wait(); err != nil {
			emit(agent.Event{Type: agent.EventError, Text: fmt.Sprintf("OpenCode CLI failed: %v", err)})
			return
		}
		if failed {
			return
		}
		if !finished {
			emit(agent.Event{Type: agent.EventError, Text: "OpenCode CLI ended without completing the turn"})
			return
		}
		emit(agent.Event{Type: agent.EventDone, Usage: &total})
	}()
	return out, nil
}
