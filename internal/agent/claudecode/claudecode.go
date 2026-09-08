// Package claudecode drives the locally installed Claude Code CLI.
//
// agenttik never touches subscription credentials: the CLI authenticates with
// whatever account is already configured in ~/.claude.
package claudecode

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

	"github.com/pausan/agenttik/internal/agent"
)

// Binary is the CLI we shell out to. A variable so tests can point at a fake.
var Binary = "claude"

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string        { return "claude" }
func (p *Provider) DisplayName() string { return "Claude Code" }

// Models are aliases rather than pinned ids, so they follow the latest release
// without a code change here.
func (p *Provider) Models() []agent.Model {
	return []agent.Model{
		{ID: "fable", Label: "Fable"},
		{ID: "opus", Label: "Opus"},
		{ID: "sonnet", Label: "Sonnet"},
		{ID: "haiku", Label: "Haiku"},
	}
}

func (p *Provider) Efforts() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

func (p *Provider) Available() error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("%s not found on PATH: %w", Binary, err)
	}
	return nil
}

func permissionFlag(p agent.Permission) []string {
	switch p.Valid() {
	case agent.PermissionPlan:
		return []string{"--permission-mode", "plan"}
	case agent.PermissionFull:
		return []string{"--dangerously-skip-permissions"}
	default:
		return []string{"--permission-mode", "acceptEdits"}
	}
}

// buildArgs is separated from Run so the flag mapping can be unit tested.
func buildArgs(req agent.TurnRequest) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	args = append(args, permissionFlag(req.Permission)...)
	if req.ProviderSessionID != "" {
		args = append(args, "--resume", req.ProviderSessionID)
	} else if req.SessionID != "" {
		// First turn: we pick the id so ours and the CLI's agree.
		args = append(args, "--session-id", req.SessionID)
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

// parse turns the CLI's JSONL stream into provider-neutral events.
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

// readLine reads one line of any length, which matters because tool results
// arrive as single very long JSON lines.
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
	case "system":
		if env.Subtype == "init" && env.SessionID != "" {
			out <- agent.Event{Type: agent.EventSessionStarted, ProviderSessionID: env.SessionID}
		}
	case "stream_event":
		handleStreamEvent(env.Event, out)
	case "assistant":
		// Text already arrived as deltas; take only tool calls from here.
		for _, b := range env.Message.Content {
			if b.Type == "tool_use" {
				out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
					ID: b.ID, Name: b.Name, Input: string(b.Input)}}
			}
		}
	case "user":
		for _, b := range env.Message.Content {
			if b.Type == "tool_result" {
				out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
					ID: b.ToolUseID, Output: flatten(b.Content), IsError: b.IsError}}
			}
		}
	case "result":
		if env.IsError {
			msg := env.Result
			if msg == "" {
				msg = env.Subtype
			}
			out <- agent.Event{Type: agent.EventError, Text: msg}
		}
		out <- agent.Event{Type: agent.EventDone, Usage: &agent.Usage{
			InputTokens:      env.Usage.InputTokens,
			OutputTokens:     env.Usage.OutputTokens,
			CacheReadTokens:  env.Usage.CacheReadInputTokens,
			CacheWriteTokens: env.Usage.CacheCreationInputTokens,
			CostUSD:          env.TotalCostUSD,
		}}
	}
}

func handleStreamEvent(ev *streamEvent, out chan<- agent.Event) {
	if ev == nil || ev.Type != "content_block_delta" || ev.Delta == nil {
		return
	}
	switch ev.Delta.Type {
	case "text_delta":
		if ev.Delta.Text != "" {
			out <- agent.Event{Type: agent.EventText, Text: ev.Delta.Text}
		}
	case "thinking_delta":
		if ev.Delta.Thinking != "" {
			out <- agent.Event{Type: agent.EventThinking, Text: ev.Delta.Thinking}
		}
	}
}

// flatten renders a tool_result body, which is either a plain string or an
// array of content blocks.
func flatten(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, blk := range blocks {
			b.WriteString(blk.Text)
		}
		return b.String()
	}
	return string(raw)
}
