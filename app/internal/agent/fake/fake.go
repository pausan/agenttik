// Package fake is a scriptable Provider used only by the end-to-end test
// suite. A turn spawns no process and reaches no account: it is driven
// entirely by directive lines in the prompt, so a browser test can make an
// agent stream text, call a tool, error out, or hang until stopped, without a
// CLI on PATH or a subscription to spend.
//
// It is registered only when Enabled reports true — see AGENTTIK_FAKE_PROVIDER
// — which the end-to-end harness sets and nothing else does, so it never
// appears in a normal build or a release.
package fake

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Enabled reports whether this process should register the fake provider.
func Enabled() bool {
	return os.Getenv("AGENTTIK_FAKE_PROVIDER") != ""
}

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string        { return "fake" }
func (p *Provider) DisplayName() string { return "Fake" }

// Models offers two, so a test can exercise switching between them the same
// way it would Claude Code's or Codex's.
func (p *Provider) Models() []agent.Model {
	return []agent.Model{
		{ID: "fake-quick", Label: "Fake Quick", ContextWindow: 128_000, Efforts: []string{"low", "high"}},
		{ID: "fake-careful", Label: "Fake Careful", ContextWindow: 128_000},
	}
}

func (p *Provider) Efforts() []string { return []string{"low", "medium", "high"} }

// TitleModel makes the fake provider also stand in for the background title
// refinement in 020-task-titles.md: the placeholder first line is replaced
// with titleReply shortly after a turn starts.
func (p *Provider) TitleModel() (model, effort string) { return "fake-quick", "low" }

// Available never fails: the fake provider has no CLI to be missing.
func (p *Provider) Available() error { return nil }

// SubscriptionLimits answers a fixed, made-up allowance, so the prompt bar's
// bars and the reset copy under them have something deterministic to show.
func (p *Provider) SubscriptionLimits(ctx context.Context) ([]agent.RateLimit, error) {
	now := time.Now()
	return []agent.RateLimit{{
		LimitID:   "fake",
		LimitName: "Fake",
		PlanType:  "Fake plan",
		Primary: &agent.RateLimitWindow{
			UsedPercent:        42,
			WindowDurationMins: 5 * 60,
			ResetsAt:           now.Add(2 * time.Hour).Unix(),
		},
		Secondary: &agent.RateLimitWindow{
			Label:       "Weekly",
			UsedPercent: 7,
			ResetsAt:    now.Add(5 * 24 * time.Hour).Unix(),
		},
	}}, nil
}

func (p *Provider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	events := make(chan agent.Event, 64)
	go func() {
		defer close(events)
		if req.Isolated {
			runTitle(ctx, req, events)
			return
		}
		runScript(ctx, req, events)
	}()
	return events, nil
}

// runTitle answers an isolated title request almost instantly, with a reply
// that still names what was actually asked — "Refined: " plus the subject
// runner.titlePrompt wrapped — so two tasks started from different prompts
// keep two different refined titles instead of converging on one constant.
func runTitle(ctx context.Context, req agent.TurnRequest, events chan<- agent.Event) {
	if !sleepCtx(ctx, 30*time.Millisecond) {
		return
	}
	events <- agent.Event{Type: agent.EventText, Text: "Refined: " + titleSubject(req.Prompt)}
}

// titleSubject reads the original prompt back out of runner.titlePrompt's
// wrapper. A wrapper it does not recognise — the contract changed, or this
// runs against some other caller entirely — is not fatal: the whole prompt
// stands in, still visibly different from the placeholder it replaces.
func titleSubject(wrapped string) string {
	const open, close = "<user-request>\n", "\n</user-request>"
	start := strings.Index(wrapped, open)
	end := strings.LastIndex(wrapped, close)
	if start < 0 || end < 0 || end < start {
		return strings.TrimSpace(wrapped)
	}
	inner := wrapped[start+len(open) : end]
	return strings.TrimSpace(strings.SplitN(inner, "\n", 2)[0])
}

var sessionCounter int64

func nextSessionID() string {
	return fmt.Sprintf("fake-%d", atomic.AddInt64(&sessionCounter, 1))
}

// runScript drives one real turn. The prompt is read line by line: a line
// whose first non-blank character is "@" is a directive, and everything else
// is the reply. Three directives exist:
//
//	@wait <ms>      pause, cut short the moment Stop cancels the turn
//	@tool <name> <input...>   a tool call and its canned result
//	@error <message>          end the turn in error; a message containing an
//	                          outage phrase ("rate limit", "connection
//	                          refused", ...) exercises the retry queue exactly
//	                          as a real provider's outage would
//
// Any line whose leading word is not one of these is treated as reply text,
// not a directive — so a prompt can still contain a literal "@" without being
// swallowed. The reply, if any, streams back word by word.
func runScript(ctx context.Context, req agent.TurnRequest, events chan<- agent.Event) {
	id := req.ProviderSessionID
	if id == "" {
		id = nextSessionID()
	}
	events <- agent.Event{Type: agent.EventSessionStarted, ProviderSessionID: id}

	var reply []string
	var toolSeq int
	for _, line := range strings.Split(req.Prompt, "\n") {
		name, rest, ok := cutDirective(line)
		if !ok {
			reply = append(reply, line)
			continue
		}
		switch name {
		case "wait":
			ms, _ := strconv.Atoi(strings.TrimSpace(rest))
			if !sleepCtx(ctx, time.Duration(ms)*time.Millisecond) {
				return
			}
		case "tool":
			toolSeq++
			id := fmt.Sprintf("tool-%d", toolSeq)
			toolName, input, _ := strings.Cut(strings.TrimSpace(rest), " ")
			events <- agent.Event{Type: agent.EventToolUse, Tool: &agent.ToolEvent{ID: id, Name: toolName, Input: input}}
			if !sleepCtx(ctx, 20*time.Millisecond) {
				return
			}
			events <- agent.Event{Type: agent.EventToolResult, Tool: &agent.ToolEvent{ID: id, Output: toolName + " ok"}}
		case "error":
			events <- agent.Event{Type: agent.EventError, Text: strings.TrimSpace(rest)}
			return
		default:
			reply = append(reply, line)
		}
	}

	text := strings.TrimSpace(strings.Join(reply, "\n"))
	if text != "" {
		for _, word := range strings.Fields(text) {
			if !sleepCtx(ctx, 15*time.Millisecond) {
				return
			}
			events <- agent.Event{Type: agent.EventText, Text: word + " "}
		}
	}

	events <- agent.Event{Type: agent.EventUsage, Usage: &agent.Usage{ContextTokens: int64(len(req.Prompt))}}
	events <- agent.Event{Type: agent.EventDone, Usage: &agent.Usage{
		InputTokens: 42, OutputTokens: 17, CacheReadTokens: 5, CacheWriteTokens: 3,
		CostUSD: 0.0042, ContextTokens: int64(len(req.Prompt)), ContextWindow: 128_000,
	}}
}

// cutDirective reports whether line is one of the three known directives and
// splits it into its name and the rest of the line. A line that only looks
// like a directive (an unrecognised word after "@") is not one: ok is false
// and the caller keeps it as reply text.
func cutDirective(line string) (name, rest string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "@") {
		return "", "", false
	}
	name, rest, _ = strings.Cut(trimmed[1:], " ")
	switch name {
	case "wait", "tool", "error":
		return name, rest, true
	default:
		return "", "", false
	}
}

// sleepCtx waits for d and reports true, or stops early and reports false the
// moment ctx is cancelled — the same way Stop cuts short a real CLI process.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
