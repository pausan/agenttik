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
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/process"
)

// Normal Codex turns use app-server instead of `codex exec --json`.
// exec reports one aggregate for the entire turn; app-server reports usage
// per thread and includes the last model request separately. That distinction
// is what keeps delegated work out of the main context gauge while retaining
// it in the task totals.
func (p *Provider) runAppServer(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	cmd := exec.Command(Binary, "app-server", "--stdio")
	cmd.Dir = req.WorkDir
	cmd.Env = agent.HomeEnv(HomeVar, req.AccountHome)
	process.Configure(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s app-server: %w", Binary, err)
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
				process.Terminate(cmd.Process)
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					process.Kill(cmd.Process)
				}
			case <-done:
			}
		}()

		serveErr := serveAppServer(ctx, req, stdin, stdout, events)
		_ = stdin.Close()
		// app-server is a long-lived host. Once this turn is complete there is
		// nothing left to read, so stop its process group instead of leaving a
		// host behind for every task turn.
		if ctx.Err() == nil {
			process.Terminate(cmd.Process)
		}
		waitErr := cmd.Wait()
		stop()

		if ctx.Err() != nil {
			return
		}
		if serveErr != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" {
				serveErr = fmt.Errorf("%w: %s", serveErr, msg)
			}
			events <- agent.Event{Type: agent.EventError, Text: serveErr.Error()}
		} else if waitErr != nil {
			// A deliberate SIGTERM after turn/completed is expected.
			return
		}
	}()
	return events, nil
}

type appRPCMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type appServerRun struct {
	req         agent.TurnRequest
	out         chan<- agent.Event
	enc         *json.Encoder
	rootID      string
	tracker     appUsageTracker
	streamed    map[string]bool
	activeTurns map[string]string
	lastErr     string
}

func serveAppServer(
	ctx context.Context,
	req agent.TurnRequest,
	stdin io.Writer,
	stdout io.Reader,
	out chan<- agent.Event,
) error {
	run := &appServerRun{
		req: req, out: out, enc: json.NewEncoder(stdin),
		tracker: appUsageTracker{threads: make(map[string]*appTrackedUsage),
			subagents: make(map[string]struct{})},
		streamed:    make(map[string]bool),
		activeTurns: make(map[string]string),
	}
	if err := run.send("initialize", 1, map[string]any{
		"clientInfo": map[string]string{
			"name": "agenttik", "title": "Agenttik", "version": "0.1",
		},
	}); err != nil {
		return err
	}

	br := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			var message appRPCMessage
			if json.Unmarshal(line, &message) == nil {
				complete, handleErr := run.handle(message)
				if handleErr != nil {
					return handleErr
				}
				if complete {
					return nil
				}
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err == io.EOF {
				return fmt.Errorf("app-server closed before the turn completed")
			}
			return fmt.Errorf("read app-server: %w", err)
		}
	}
}

func (r *appServerRun) send(method string, id int, params any) error {
	message := map[string]any{"method": method, "id": id}
	if params != nil {
		message["params"] = params
	}
	if err := r.enc.Encode(message); err != nil {
		return fmt.Errorf("write app-server %s: %w", method, err)
	}
	return nil
}

func (r *appServerRun) notify(method string, params any) error {
	message := map[string]any{"method": method}
	if params != nil {
		message["params"] = params
	}
	if err := r.enc.Encode(message); err != nil {
		return fmt.Errorf("write app-server %s: %w", method, err)
	}
	return nil
}

func (r *appServerRun) handle(message appRPCMessage) (bool, error) {
	if message.Method != "" {
		if hasRPCID(message.ID) {
			return false, r.replyToServerRequest(message)
		}
		return r.handleNotification(message.Method, message.Params)
	}

	id, ok := rpcIntID(message.ID)
	if !ok {
		return false, nil
	}
	if message.Error != nil {
		return false, fmt.Errorf("app-server request %d: %s", id, message.Error.Message)
	}
	switch id {
	case 1:
		if err := r.notify("initialized", map[string]any{}); err != nil {
			return false, err
		}
		method, params := threadRequest(r.req)
		return false, r.send(method, 2, params)
	case 2:
		var response struct {
			Thread appServerThread `json:"thread"`
		}
		if err := json.Unmarshal(message.Result, &response); err != nil {
			return false, fmt.Errorf("decode app-server thread: %w", err)
		}
		if response.Thread.ID == "" {
			return false, fmt.Errorf("app-server returned an empty thread id")
		}
		r.setRoot(response.Thread.ID)
		r.out <- agent.Event{
			Type: agent.EventSessionStarted, ProviderSessionID: response.Thread.ID,
		}
		return false, r.send("turn/start", 3, turnRequest(r.req, response.Thread.ID))
	case 3:
		var response struct {
			Turn appServerTurn `json:"turn"`
		}
		if json.Unmarshal(message.Result, &response) == nil && response.Turn.ID != "" {
			r.activeTurns[r.rootID] = response.Turn.ID
		}
		return false, nil
	default:
		return false, nil
	}
}

func threadRequest(req agent.TurnRequest) (string, map[string]any) {
	params := map[string]any{
		"cwd":            req.WorkDir,
		"approvalPolicy": "never",
		"sandbox":        sandboxFlag(req.Permission),
	}
	if req.Model != "" {
		params["model"] = req.Model
	}
	if req.ProviderSessionID != "" {
		params["threadId"] = req.ProviderSessionID
		params["excludeTurns"] = true
		return "thread/resume", params
	}
	params["ephemeral"] = false
	return "thread/start", params
}

func turnRequest(req agent.TurnRequest, threadID string) map[string]any {
	params := map[string]any{
		"threadId": threadID,
		"input": []any{map[string]any{
			"type": "text", "text": req.Prompt, "text_elements": []any{},
		}},
		"cwd":            req.WorkDir,
		"approvalPolicy": "never",
	}
	if req.Model != "" {
		params["model"] = req.Model
	}
	if req.Effort != "" {
		params["effort"] = req.Effort
	}
	return params
}

func hasRPCID(raw json.RawMessage) bool {
	return len(raw) != 0 && string(raw) != "null"
}

func rpcIntID(raw json.RawMessage) (int, bool) {
	if !hasRPCID(raw) {
		return 0, false
	}
	var id int
	return id, json.Unmarshal(raw, &id) == nil
}

func (r *appServerRun) replyToServerRequest(message appRPCMessage) error {
	var result any
	switch message.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval",
		"applyPatchApproval", "execCommandApproval":
		result = map[string]any{"decision": "decline"}
	case "item/tool/requestUserInput":
		// There is no interactive approval bridge in the web task runner.
		// An empty answer lets the model continue and explain what it needs.
		result = map[string]any{"answers": map[string]any{}}
	case "mcpServer/elicitation/request":
		result = map[string]any{"action": "decline", "content": nil, "_meta": nil}
	default:
		return r.enc.Encode(map[string]any{
			"id": message.ID,
			"error": map[string]any{
				"code": -32601, "message": "client method not supported",
			},
		})
	}
	return r.enc.Encode(map[string]any{"id": message.ID, "result": result})
}

type appServerThread struct {
	ID             string `json:"id"`
	ParentThreadID string `json:"parentThreadId"`
}

type appServerNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`

	Thread     appServerThread     `json:"thread"`
	TokenUsage appThreadTokenUsage `json:"tokenUsage"`
	Item       appServerItem       `json:"item"`
	Turn       appServerTurn       `json:"turn"`

	Error     appServerTurnError `json:"error"`
	WillRetry bool               `json:"willRetry"`
}

type appServerTurn struct {
	ID     string              `json:"id"`
	Status string              `json:"status"`
	Error  *appServerTurnError `json:"error"`
}

type appServerTurnError struct {
	Message string `json:"message"`
}

type appServerItem struct {
	Type             string          `json:"type"`
	ID               string          `json:"id"`
	Text             string          `json:"text"`
	Summary          []string        `json:"summary"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregatedOutput"`
	ExitCode         *int            `json:"exitCode"`
	Status           string          `json:"status"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Arguments        json.RawMessage `json:"arguments"`
	Result           json.RawMessage `json:"result"`
	Error            *struct {
		Message string `json:"message"`
	} `json:"error"`
	Query             string   `json:"query"`
	ReceiverThreadIDs []string `json:"receiverThreadIds"`
	AgentThreadID     string   `json:"agentThreadId"`
	Prompt            string   `json:"prompt"`
}

func (r *appServerRun) handleNotification(
	method string,
	raw json.RawMessage,
) (bool, error) {
	var params appServerNotification
	if err := json.Unmarshal(raw, &params); err != nil {
		return false, nil
	}
	switch method {
	case "thread/started":
		if params.Thread.ID != "" && params.Thread.ParentThreadID == "" && r.rootID == "" {
			r.setRoot(params.Thread.ID)
		} else if params.Thread.ParentThreadID != "" {
			r.tracker.markSubagent(params.Thread.ID)
		}
	case "turn/started":
		if params.ThreadID != "" && params.Turn.ID != "" {
			r.activeTurns[params.ThreadID] = params.Turn.ID
		}
	case "thread/tokenUsage/updated":
		if params.ThreadID == "" {
			return false, nil
		}
		// A resumed root or child may volunteer existing totals while it
		// loads. Only usage tagged with that thread's newly started turn is
		// part of this task; seed any earlier notification as history.
		activeTurnID := r.activeTurns[params.ThreadID]
		if activeTurnID == "" || activeTurnID != params.TurnID {
			r.tracker.seed(params.ThreadID, params.TokenUsage)
			return false, nil
		}
		contextTokens, contextWindow, root := r.tracker.update(params.ThreadID, params.TokenUsage)
		if root && contextTokens > 0 {
			r.out <- agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{
				ContextTokens: contextTokens, ContextWindow: contextWindow,
			}}
		}
	case "item/agentMessage/delta":
		if params.ThreadID == r.rootID && params.Delta != "" {
			r.streamed[params.ItemID] = true
			r.out <- agent.Event{Type: agent.EventText, Text: params.Delta}
		}
	case "item/reasoning/summaryTextDelta":
		if params.ThreadID == r.rootID && params.Delta != "" {
			r.streamed[params.ItemID] = true
			r.out <- agent.Event{Type: agent.EventThinking, Text: params.Delta}
		}
	case "item/started":
		r.handleItem(params.ThreadID, params.Item, false)
	case "item/completed":
		r.handleItem(params.ThreadID, params.Item, true)
	case "error":
		if params.ThreadID == r.rootID && !params.WillRetry {
			r.lastErr = params.Error.Message
		}
	case "turn/completed":
		if params.ThreadID != r.rootID {
			return false, nil
		}
		message := r.lastErr
		if params.Turn.Error != nil && params.Turn.Error.Message != "" {
			message = params.Turn.Error.Message
		}
		if params.Turn.Status == "failed" && message == "" {
			message = "Codex turn failed"
		}
		if params.Turn.Status == "interrupted" && message == "" {
			message = "Codex turn interrupted"
		}
		if message != "" {
			r.out <- agent.Event{Type: agent.EventError, Text: message}
		}
		usage := r.tracker.usage()
		r.out <- agent.Event{Type: agent.EventDone, Usage: &usage}
		return true, nil
	}
	return false, nil
}

func (r *appServerRun) setRoot(threadID string) {
	r.rootID = threadID
	r.tracker.rootID = threadID
}

func (r *appServerRun) handleItem(threadID string, item appServerItem, completed bool) {
	if threadID != r.rootID {
		return
	}
	for _, childID := range item.ReceiverThreadIDs {
		r.tracker.markSubagent(childID)
	}
	if item.AgentThreadID != "" {
		r.tracker.markSubagent(item.AgentThreadID)
	}

	if !completed {
		switch item.Type {
		case "commandExecution":
			r.out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
				ID: item.ID, Name: "shell", Input: item.Command,
			}}
		case "mcpToolCall", "dynamicToolCall":
			r.out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
				ID: item.ID, Name: appToolName(item), Input: string(item.Arguments),
			}}
		case "webSearch":
			r.out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
				ID: item.ID, Name: "web search", Input: item.Query,
			}}
		case "collabAgentToolCall":
			r.out <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{
				ID: item.ID, Name: "subagent " + item.Tool, Input: item.Prompt,
			}}
		}
		return
	}

	switch item.Type {
	case "agentMessage":
		if item.Text != "" && !r.streamed[item.ID] {
			r.out <- agent.Event{Type: agent.EventText, Text: item.Text}
		}
	case "reasoning":
		if !r.streamed[item.ID] {
			if text := strings.Join(item.Summary, ""); text != "" {
				r.out <- agent.Event{Type: agent.EventThinking, Text: text}
			}
		}
	case "commandExecution":
		r.out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
			ID: item.ID, Output: item.AggregatedOutput, IsError: appItemFailed(item),
		}}
	case "mcpToolCall", "dynamicToolCall", "webSearch":
		r.out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
			ID: item.ID, Output: appToolOutput(item), IsError: appItemFailed(item),
		}}
	case "collabAgentToolCall":
		r.out <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{
			ID: item.ID, Output: item.Status, IsError: appItemFailed(item),
		}}
	}
}

func appToolName(item appServerItem) string {
	if item.Server != "" && item.Tool != "" {
		return item.Server + "." + item.Tool
	}
	if item.Tool != "" {
		return item.Tool
	}
	return "tool"
}

func appToolOutput(item appServerItem) string {
	if item.Error != nil && item.Error.Message != "" {
		return item.Error.Message
	}
	if len(item.Result) == 0 || string(item.Result) == "null" {
		return ""
	}
	return string(item.Result)
}

func appItemFailed(item appServerItem) bool {
	return item.Status == "failed" || item.ExitCode != nil && *item.ExitCode != 0 ||
		item.Error != nil
}

type appTokenBreakdown struct {
	InputTokens           int64 `json:"inputTokens"`
	CachedInputTokens     int64 `json:"cachedInputTokens"`
	CacheWriteInputTokens int64 `json:"cacheWriteInputTokens"`
	OutputTokens          int64 `json:"outputTokens"`
}

func (u appTokenBreakdown) minus(base appTokenBreakdown) appTokenBreakdown {
	return appTokenBreakdown{
		InputTokens:           nonnegative(u.InputTokens - base.InputTokens),
		CachedInputTokens:     nonnegative(u.CachedInputTokens - base.CachedInputTokens),
		CacheWriteInputTokens: nonnegative(u.CacheWriteInputTokens - base.CacheWriteInputTokens),
		OutputTokens:          nonnegative(u.OutputTokens - base.OutputTokens),
	}
}

func (u appTokenBreakdown) hasTokens() bool {
	return u.InputTokens != 0 || u.CachedInputTokens != 0 ||
		u.CacheWriteInputTokens != 0 || u.OutputTokens != 0
}

func nonnegative(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}

type appThreadTokenUsage struct {
	Total              appTokenBreakdown `json:"total"`
	Last               appTokenBreakdown `json:"last"`
	ModelContextWindow *int64            `json:"modelContextWindow"`
}

type appTrackedUsage struct {
	baseline appTokenBreakdown
	current  appTokenBreakdown
	last     appTokenBreakdown
	window   int64
	seen     bool
}

type appUsageTracker struct {
	rootID    string
	threads   map[string]*appTrackedUsage
	subagents map[string]struct{}
}

func (t *appUsageTracker) seed(threadID string, usage appThreadTokenUsage) {
	tracked := &appTrackedUsage{
		baseline: usage.Total,
		current:  usage.Total,
		last:     usage.Last,
		seen:     true,
	}
	if usage.ModelContextWindow != nil {
		tracked.window = *usage.ModelContextWindow
	}
	t.threads[threadID] = tracked
}

// update establishes a resumed thread's pre-turn baseline from total-last on
// its first notification. For a new root or child that expression is simply
// zero. Subsequent notifications then accumulate every model request in this
// task without importing earlier turns.
func (t *appUsageTracker) update(
	threadID string,
	usage appThreadTokenUsage,
) (contextTokens, contextWindow int64, root bool) {
	tracked := t.threads[threadID]
	if tracked == nil {
		tracked = &appTrackedUsage{}
		t.threads[threadID] = tracked
	}
	if !tracked.seen {
		tracked.baseline = usage.Total.minus(usage.Last)
		tracked.seen = true
	}
	tracked.current = usage.Total
	tracked.last = usage.Last
	if usage.ModelContextWindow != nil {
		tracked.window = *usage.ModelContextWindow
	}
	if threadID != t.rootID {
		t.markSubagent(threadID)
		return 0, 0, false
	}
	return tracked.last.InputTokens, tracked.window, true
}

func (t *appUsageTracker) markSubagent(threadID string) {
	if threadID != "" && threadID != t.rootID {
		t.subagents[threadID] = struct{}{}
	}
}

func (t *appUsageTracker) usage() agent.Usage {
	result := agent.Usage{UsageBreakdown: true, SubagentCount: int64(len(t.subagents))}
	for threadID, tracked := range t.threads {
		delta := tracked.current.minus(tracked.baseline)
		if threadID == t.rootID {
			result.MainInputTokens += delta.InputTokens
			result.MainOutputTokens += delta.OutputTokens
			result.MainCacheReadTokens += delta.CachedInputTokens
			result.MainCacheWriteTokens += delta.CacheWriteInputTokens
			result.ContextTokens = tracked.last.InputTokens
			result.ContextWindow = tracked.window
		} else {
			result.SubagentInputTokens += delta.InputTokens
			result.SubagentOutputTokens += delta.OutputTokens
			result.SubagentCacheReadTokens += delta.CachedInputTokens
			result.SubagentCacheWriteTokens += delta.CacheWriteInputTokens
		}
	}
	result.InputTokens = result.MainInputTokens + result.SubagentInputTokens
	result.OutputTokens = result.MainOutputTokens + result.SubagentOutputTokens
	result.CacheReadTokens = result.MainCacheReadTokens + result.SubagentCacheReadTokens
	result.CacheWriteTokens = result.MainCacheWriteTokens + result.SubagentCacheWriteTokens
	return result
}
