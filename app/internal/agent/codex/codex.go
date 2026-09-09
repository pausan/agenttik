// Package codex drives the locally installed Codex CLI, using the ChatGPT
// account already configured in ~/.codex.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

var Binary = "codex"

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string        { return "codex" }
func (p *Provider) DisplayName() string { return "Codex" }

func (p *Provider) Models() []agent.Model {
	return []agent.Model{
		{ID: "gpt-5-codex", Label: "GPT-5 Codex", ContextWindow: 400_000},
		{ID: "gpt-5", Label: "GPT-5", ContextWindow: 400_000},
	}
}

func (p *Provider) Efforts() []string {
	return []string{"minimal", "low", "medium", "high"}
}

func (p *Provider) Available() error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("%s not found on PATH: %w", Binary, err)
	}
	return nil
}

func sandboxFlag(p agent.Permission) string {
	switch p.Valid() {
	case agent.PermissionPlan:
		return "read-only"
	case agent.PermissionFull:
		return "danger-full-access"
	default:
		return "workspace-write"
	}
}

// buildArgs is separated from Run so the flag mapping can be unit tested.
func buildArgs(req agent.TurnRequest) []string {
	args := []string{"exec"}
	if req.ProviderSessionID != "" {
		args = append(args, "resume", req.ProviderSessionID)
		args = append(args, "--json")
		if req.Model != "" {
			args = append(args, "--model", req.Model)
		}
		if req.Effort != "" {
			args = append(args, "-c", "model_reasoning_effort="+req.Effort)
		}
		if req.Permission.Valid() == agent.PermissionFull {
			args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		}
		// `codex exec resume` has no --cd or --sandbox flags. cmd.Dir supplies
		// the former; Codex restores the original session's sandbox policy.
		return args
	}
	args = append(args, "--json", "--cd", req.WorkDir)
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+req.Effort)
	}
	if req.Permission.Valid() == agent.PermissionFull {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else {
		args = append(args, "--sandbox", sandboxFlag(req.Permission))
	}
	return args
}

func (p *Provider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	if err := p.Available(); err != nil {
		return nil, err
	}
	cmd := exec.Command(Binary, buildArgs(req)...)
	cmd.Dir = req.WorkDir
	// The prompt goes in on stdin, never as an argv element.
	cmd.Stdin = strings.NewReader(req.Prompt)
	// Own process group so cancelling kills the CLI's children too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", Binary, err)
	}

	events := make(chan agent.Event, 64)
	go func() {
		defer close(events)

		// Kill the whole process group when the caller cancels.
		done := make(chan struct{})
		var once sync.Once
		stop := func() { once.Do(func() { close(done) }) }
		go func() {
			select {
			case <-ctx.Done():
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
			case <-done:
			}
		}()
		defer stop()

		parse(stdout, events)

		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			events <- agent.Event{Type: agent.EventError, Text: msg}
		}
	}()
	return events, nil
}

// parse turns codex exec --json's JSONL stream into provider-neutral events.
func parse(r io.Reader, out chan<- agent.Event) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			handleLine(line, out)
		}
		if err != nil {
			return
		}
	}
}

// readLine reads one line of any length, which matters because command output
// can arrive as one large JSON line.
func readLine(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		buf = append(buf, chunk...)
		if err != nil {
			return buf, err
		}
		if !isPrefix {
			return buf, nil
		}
	}
}

func handleLine(line []byte, out chan<- agent.Event) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		// Non-JSON noise on stdout is not fatal; skip it.
		return
	}

	switch env.Type {
	case "thread.started":
		if env.ThreadID != "" {
			out <- agent.Event{Type: agent.EventSessionStarted, ProviderSessionID: env.ThreadID}
		}
	case "item.started":
		handleItemStarted(env.Item, out)
	case "item.completed":
		handleItemCompleted(env.Item, out)
	case "turn.completed":
		if env.Usage != nil {
			usage := env.Usage.agentUsage()
			out <- agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{ContextTokens: usage.ContextTokens}}
			out <- agent.Event{Type: agent.EventDone, Usage: &usage}
		} else {
			out <- agent.Event{Type: agent.EventDone}
		}
	case "turn.failed", "error":
		if msg := env.errorText(); msg != "" {
			out <- agent.Event{Type: agent.EventError, Text: msg}
		}
	}
}

func handleItemStarted(item *item, out chan<- agent.Event) {
	if item == nil {
		return
	}
	switch item.Type {
	case "command_execution":
		out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
			ID: item.ID, Name: "shell", Input: item.Command}}
	case "mcp_tool_call":
		out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
			ID: item.ID, Name: item.toolName(), Input: item.toolInput()}}
	case "web_search":
		out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
			ID: item.ID, Name: "web search", Input: item.Query}}
	}
}

func handleItemCompleted(item *item, out chan<- agent.Event) {
	if item == nil {
		return
	}
	switch item.Type {
	case "agent_message":
		if item.Text != "" {
			out <- agent.Event{Type: agent.EventText, Text: item.Text}
		}
	case "reasoning":
		if text := item.reasoningText(); text != "" {
			out <- agent.Event{Type: agent.EventThinking, Text: text}
		}
	case "command_execution":
		out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
			ID: item.ID, Output: item.AggregatedOutput, IsError: item.failed()}}
	case "mcp_tool_call":
		out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
			ID: item.ID, Output: item.toolOutput(), IsError: item.failed()}}
	case "web_search":
		out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
			ID: item.ID, Output: item.toolOutput(), IsError: item.failed()}}
	}
}
