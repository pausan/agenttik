// Package claudecode drives the locally installed Claude Code CLI.
//
// agenttik never touches subscription credentials: the CLI authenticates with
// whatever account is already configured in ~/.claude.
package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/process"
)

// Binary is the CLI we shell out to. A variable so tests can point at a fake.
var Binary = "claude"

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string        { return "claude" }
func (p *Provider) DisplayName() string { return "Claude Code" }

// Models are aliases rather than pinned ids, so they follow the latest release
// without a code change here. The windows are only what to show before the
// first turn reports one: every result line names the window the CLI actually
// used, and that is what the gauge measures against from then on.
func (p *Provider) Models() []agent.Model {
	return []agent.Model{
		{ID: "fable", Label: "Fable", ContextWindow: 1_000_000},
		{ID: "opus", Label: "Opus", ContextWindow: 1_000_000},
		{ID: "sonnet", Label: "Sonnet", ContextWindow: 1_000_000},
		{ID: "haiku", Label: "Haiku", ContextWindow: 200_000},
	}
}

func (p *Provider) Efforts() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

// SmallModel is the smallest Claude model the app offers. Naming a session and
// summarising a finished one are one-shot requests, so neither needs the model
// chosen for its real turns.
func (p *Provider) SmallModel() (model, effort string) {
	return "haiku", "low"
}

func (p *Provider) Available() error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("%s not found on PATH: %w", Binary, err)
	}
	return nil
}

// permissionFlag maps our posture onto the CLI's --permission-mode.
//
// Workspace uses `auto`, not `acceptEdits`. Both auto-approve file edits, but
// acceptEdits still asks before running a command, and under `-p` there is no
// prompt to answer, so every Bash call is refused — the agent can edit files
// but never build, test, or commit. `auto` is the mode the IDE extensions use
// for their Auto setting: it approves ordinary work and keeps asking for the
// genuinely destructive things, which under `-p` means those are refused
// instead of everything.
func permissionFlag(p agent.Permission) []string {
	switch p.Valid() {
	case agent.PermissionPlan:
		return []string{"--permission-mode", "plan"}
	case agent.PermissionFull:
		return []string{"--dangerously-skip-permissions"}
	default:
		return []string{"--permission-mode", "auto"}
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
	if req.Isolated {
		args = append(args, "--no-session-persistence", "--safe-mode", "--setting-sources", "user")
		// Metadata requests cannot use tools. Conflict resolution explicitly
		// allows workspace edits while still avoiding session persistence.
		if req.Permission != agent.PermissionWorkspace && req.Permission != agent.PermissionFull {
			args = append(args, "--tools", "")
		}
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
	if err := req.CheckWorkDir(); err != nil {
		return nil, err
	}
	cmd, stdout, stderr, err := start(req)
	if err != nil {
		return nil, err
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
				process.Terminate(cmd.Process)
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					process.Kill(cmd.Process)
				}
			case <-done:
			}
		}()
		defer stop()

		parse(stdout, req.Model, events)

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

// start recreates the command once when the CLI disappears between LookPath
// and exec. Claude Code replaces its versioned executable while updating, so a
// short ENOENT window should not fail an otherwise valid turn.
func start(req agent.TurnRequest) (*exec.Cmd, io.ReadCloser, *strings.Builder, error) {
	for attempt := 0; attempt < 2; attempt++ {
		cmd := exec.Command(Binary, buildArgs(req)...)
		cmd.Dir = req.WorkDir
		// Which subscription answers the turn. Nil for the machine's own
		// login, which is exec's "inherit mine".
		cmd.Env = agent.HomeEnv(HomeVar, req.AccountHome)
		// The prompt goes in on stdin, never as an argv element.
		cmd.Stdin = strings.NewReader(req.Prompt)
		// Own process group so cancelling kills the CLI's children too.
		process.Configure(cmd)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
		}
		var stderr strings.Builder
		cmd.Stderr = &stderr

		if err := cmd.Start(); err == nil {
			return cmd, stdout, &stderr, nil
		} else if attempt == 0 && errors.Is(err, fs.ErrNotExist) {
			stdout.Close()
			continue
		} else {
			stdout.Close()
			return nil, nil, nil, fmt.Errorf("start %s: %w", Binary, err)
		}
	}
	panic("unreachable")
}

// parse turns the CLI's JSONL stream into provider-neutral events. model is
// the alias the turn asked for, which the result line's per-model breakdown is
// read against.
func parse(r io.Reader, model string, out chan<- agent.Event) {
	br := bufio.NewReaderSize(r, 64*1024)
	p := streamParser{model: model, subagents: make(map[string]struct{})}
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			p.handleLine(line, out)
		}
		if err != nil {
			return
		}
	}
}

type streamParser struct {
	model         string
	main          usage
	subagentsUsed usage
	subagents     map[string]struct{}
	contextTokens int64
	hasBreakdown  bool
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

func (p *streamParser) handleLine(line []byte, out chan<- agent.Event) {
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
	case "rate_limit_event":
		// The CLI names its own subscription allowance here. It is the only
		// place it does, and it only appears once a bucket is near its
		// ceiling, so the reading is stored and shown with its age rather
		// than expected on every turn.
		if env.RateLimitInfo != nil {
			out <- agent.Event{Type: agent.EventLimits, Limits: env.RateLimitInfo.limits()}
		}
	case "assistant":
		// Every assistant message names the prompt that produced it. Only a
		// root message moves the main context gauge; child-agent prompts have
		// independent windows and used to make this ring jump to a false 100%.
		if n := env.Message.Usage.contextTokens(); n > 0 && env.ParentToolUseID == "" {
			p.contextTokens = n
			out <- agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{ContextTokens: n}}
		}
		p.addUsage(env.ParentToolUseID, env.Message.Usage)
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
		usage := agent.Usage{
			InputTokens:              env.Usage.InputTokens,
			OutputTokens:             env.Usage.OutputTokens,
			CacheReadTokens:          env.Usage.CacheReadInputTokens,
			CacheWriteTokens:         env.Usage.CacheCreationInputTokens,
			CostUSD:                  env.TotalCostUSD,
			ContextTokens:            p.contextTokens,
			ContextWindow:            env.contextWindow(p.model),
			MainInputTokens:          p.main.InputTokens,
			MainOutputTokens:         p.main.OutputTokens,
			MainCacheReadTokens:      p.main.CacheReadInputTokens,
			MainCacheWriteTokens:     p.main.CacheCreationInputTokens,
			SubagentInputTokens:      p.subagentsUsed.InputTokens,
			SubagentOutputTokens:     p.subagentsUsed.OutputTokens,
			SubagentCacheReadTokens:  p.subagentsUsed.CacheReadInputTokens,
			SubagentCacheWriteTokens: p.subagentsUsed.CacheCreationInputTokens,
			SubagentCount:            int64(len(p.subagents)),
			UsageBreakdown:           p.hasBreakdown,
		}
		out <- agent.Event{Type: agent.EventDone, Usage: &usage}
	}
}

func (p *streamParser) addUsage(parentToolUseID string, u usage) {
	if !u.hasTokens() {
		return
	}
	p.hasBreakdown = true
	dst := &p.main
	if parentToolUseID != "" {
		dst = &p.subagentsUsed
		p.subagents[parentToolUseID] = struct{}{}
	}
	dst.InputTokens += u.InputTokens
	dst.OutputTokens += u.OutputTokens
	dst.CacheReadInputTokens += u.CacheReadInputTokens
	dst.CacheCreationInputTokens += u.CacheCreationInputTokens
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
