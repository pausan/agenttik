// Package copilot drives the locally installed GitHub Copilot CLI, using the
// Copilot account already configured for the machine.
package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/process"
)

var Binary = "copilot"

type Provider struct{ cache modelCache }

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string        { return "copilot" }
func (p *Provider) DisplayName() string { return "GitHub Copilot" }

func (p *Provider) Available() error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("%s not found on PATH: %w", Binary, err)
	}
	return nil
}

// buildArgs is separated from Run so the CLI mode and permission mapping are
// easy to inspect without starting a billable session.
func buildArgs(req agent.TurnRequest) []string {
	args := []string{
		"-p", req.Prompt,
		"--output-format", "json",
		"--stream", "on",
		"--no-auto-update",
		"--no-ask-user",
	}
	if req.ProviderSessionID != "" {
		args = append(args, "--resume="+req.ProviderSessionID)
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}

	switch req.Permission.Valid() {
	case agent.PermissionPlan:
		// Plan mode keeps the CLI from changing files. Tools still need to be
		// approved up front because prompt mode has no interactive user.
		args = append(args, "--plan", "--allow-all-tools")
	case agent.PermissionFull:
		args = append(args, "--allow-all")
	default:
		// The work directory remains the boundary for file access; Copilot's
		// non-interactive mode needs tool approval supplied on the command line.
		args = append(args, "--allow-all-tools")
		if req.WorkDir != "" {
			args = append(args, "--add-dir", req.WorkDir)
		}
	}
	if req.Isolated {
		// Title requests must not load project instructions or built-in MCP
		// servers. The prompt itself also tells the model not to use tools.
		args = append(args, "--no-custom-instructions", "--disable-builtin-mcps")
	}
	// Which subscription answers the turn. Nothing for the machine's own
	// login, so a single-subscription machine runs the command it always ran.
	return append(args, homeArgs(req.AccountHome)...)
}

func (p *Provider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	if err := p.Available(); err != nil {
		return nil, err
	}
	if err := req.CheckWorkDir(); err != nil {
		return nil, err
	}
	cmd := exec.Command(Binary, buildArgs(req)...)
	cmd.Dir = req.WorkDir
	process.Configure(cmd)

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

		done := make(chan struct{})
		var once sync.Once
		stop := func() { once.Do(func() { close(done) }) }
		go func() {
			select {
			case <-ctx.Done():
				_ = process.Terminate(cmd.Process)
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					_ = process.Kill(cmd.Process)
				}
			case <-done:
			}
		}()
		defer stop()

		parser := &streamParser{
			textDeltas:      make(map[string]bool),
			reasoningDeltas: make(map[string]bool),
		}
		parse(stdout, parser, events)

		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			events <- agent.Event{Type: agent.EventError, Text: message}
		} else if err == nil && ctx.Err() == nil && !parser.done {
			// Older CLI versions may omit session.idle in prompt mode. A clean
			// process exit still means the turn is complete.
			events <- agent.Event{Type: agent.EventDone, Usage: parser.doneUsage()}
		}
	}()
	return events, nil
}

type streamParser struct {
	textDeltas      map[string]bool
	reasoningDeltas map[string]bool
	total           agent.Usage
	hasUsage        bool
	contextTokens   int64
	contextWindow   int64
	done            bool
}

func parse(r io.Reader, parser *streamParser, out chan<- agent.Event) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			parser.handleLine(line, out)
		}
		if err != nil {
			return
		}
	}
}

// readLine accepts a large JSON event without imposing bufio.Scanner's
// default 64 KiB limit; tool arguments and results can be much larger.
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

func (p *streamParser) handleLine(line []byte, out chan<- agent.Event) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}

	switch env.Type {
	case "session.start":
		var data sessionStart
		if unmarshalData(env.Data, &data) && data.SessionID != "" {
			out <- agent.Event{Type: agent.EventSessionStarted, ProviderSessionID: data.SessionID}
		}
	case "session.error":
		var data sessionError
		if unmarshalData(env.Data, &data) && data.Message != "" {
			out <- agent.Event{Type: agent.EventError, Text: data.Message}
		}
	case "session.usage_info":
		var data sessionUsageInfo
		if !unmarshalData(env.Data, &data) {
			return
		}
		p.contextTokens, p.contextWindow = data.CurrentTokens, data.TokenLimit
		out <- agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{
			ContextTokens: data.CurrentTokens, ContextWindow: data.TokenLimit,
		}}
	case "assistant.message_delta":
		var data assistantMessageDelta
		if !unmarshalData(env.Data, &data) {
			return
		}
		p.textDeltas[data.MessageID] = true
		if data.DeltaContent != "" {
			out <- agent.Event{Type: agent.EventText, Text: data.DeltaContent}
		}
	case "assistant.message":
		var data assistantMessage
		if !unmarshalData(env.Data, &data) {
			return
		}
		// JSON output can contain either deltas or the completed message,
		// depending on the CLI version. Do not append both.
		if data.Content != "" && !p.textDeltas[data.MessageID] {
			out <- agent.Event{Type: agent.EventText, Text: data.Content}
		}
		if data.ReasoningText != "" {
			out <- agent.Event{Type: agent.EventThinking, Text: data.ReasoningText}
		}
	case "assistant.reasoning_delta":
		var data assistantReasoningDelta
		if !unmarshalData(env.Data, &data) {
			return
		}
		p.reasoningDeltas[data.ReasoningID] = true
		if data.DeltaContent != "" {
			out <- agent.Event{Type: agent.EventThinking, Text: data.DeltaContent}
		}
	case "assistant.reasoning":
		var data assistantReasoning
		if !unmarshalData(env.Data, &data) {
			return
		}
		if data.Content != "" && !p.reasoningDeltas[data.ReasoningID] {
			out <- agent.Event{Type: agent.EventThinking, Text: data.Content}
		}
	case "assistant.usage":
		var data assistantUsage
		if !unmarshalData(env.Data, &data) {
			return
		}
		usage := data.usage()
		p.total.InputTokens += usage.InputTokens
		p.total.OutputTokens += usage.OutputTokens
		p.total.CacheReadTokens += usage.CacheReadTokens
		p.total.CacheWriteTokens += usage.CacheWriteTokens
		p.total.CostUSD += usage.CostUSD
		p.hasUsage = true
		if usage.ContextTokens > 0 {
			out <- agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{
				ContextTokens: usage.ContextTokens,
			}}
		}
		if limits := publicQuotaLimits(data.QuotaSnapshots); len(limits) > 0 {
			out <- agent.Event{Type: agent.EventLimits, Limits: limits}
		}
	case "tool.execution_start":
		var data toolExecutionStart
		if !unmarshalData(env.Data, &data) {
			return
		}
		name := data.ToolName
		if name == "" {
			name = data.MCPServerName
		}
		if name == "" {
			name = "tool"
		}
		out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
			ID: data.ToolCallID, Name: name, Input: rawText(data.Arguments),
		}}
	case "tool.execution_complete":
		var data toolExecutionComplete
		if !unmarshalData(env.Data, &data) {
			return
		}
		output := rawText(data.Result)
		isError := !data.Success
		if len(data.Error) > 0 && string(data.Error) != "null" {
			isError = true
			if output == "" {
				output = rawText(data.Error)
			}
		}
		out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
			ID: data.ToolCallID, Output: output, IsError: isError,
		}}
	case "session.idle":
		p.done = true
		out <- agent.Event{Type: agent.EventDone, Usage: p.doneUsage()}
	}
}

func (u assistantUsage) usage() agent.Usage {
	return agent.Usage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		// One AI credit is $0.01 and Copilot reports nano-AIU.
		CostUSD:       u.CopilotUsage.TotalNanoAIU / 100_000_000_000,
		ContextTokens: u.InputTokens,
	}
}

func (p *streamParser) doneUsage() *agent.Usage {
	if !p.hasUsage && p.contextTokens == 0 && p.contextWindow == 0 {
		return nil
	}
	u := p.total
	u.ContextTokens, u.ContextWindow = p.contextTokens, p.contextWindow
	return &u
}
