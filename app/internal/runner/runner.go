// Package runner owns the turn lifecycle: spawn a provider, persist what it
// produces, and fan events out to the UI.
package runner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

var (
	ErrBusy            = errors.New("session already has a turn running")
	ErrNotRunning      = errors.New("session has no running turn")
	ErrUnknownProvider = errors.New("unknown provider")
)

// Event is what the UI receives over SSE: a provider event tagged with the
// session it belongs to, plus the ids needed to reconcile with stored rows.
type Event struct {
	SessionID string      `json:"session_id"`
	TurnID    int64       `json:"turn_id,omitempty"`
	Event     agent.Event `json:"event"`
	// Stats is attached to the done event so the panel can refresh without
	// a second request.
	Stats *store.Stats `json:"stats,omitempty"`
}

type Runner struct {
	store    *store.Store
	registry *agent.Registry
	hub      *Hub

	mu     sync.Mutex
	active map[string]context.CancelFunc // session id -> cancel
}

func New(s *store.Store, reg *agent.Registry, hub *Hub) *Runner {
	return &Runner{store: s, registry: reg, hub: hub,
		active: make(map[string]context.CancelFunc)}
}

func (r *Runner) Hub() *Hub { return r.hub }

// ProjectTopic is the hub topic that receives the done event of every turn
// run in the project, so a project view can refresh its totals while several
// sessions run at once.
func ProjectTopic(projectID int64) string { return "project:" + strconv.FormatInt(projectID, 10) }

// Running reports whether a turn is in flight for the session.
func (r *Runner) Running(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.active[sessionID]
	return ok
}

// Send starts a turn. It returns as soon as the provider process is up; the
// turn continues in the background and is observed over the hub.
func (r *Runner) Send(sessionID, prompt string) (*store.Turn, error) {
	sess, err := r.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	provider, ok := r.registry.Get(sess.Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, sess.Provider)
	}

	r.mu.Lock()
	if _, busy := r.active[sessionID]; busy {
		r.mu.Unlock()
		return nil, ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.active[sessionID] = cancel
	r.mu.Unlock()

	release := func() {
		r.mu.Lock()
		delete(r.active, sessionID)
		r.mu.Unlock()
		cancel()
	}

	// The first prompt names the session.
	if strings.TrimSpace(sess.Title) == "" {
		title := titleFrom(prompt)
		if err := r.store.SetSessionTitle(sessionID, title); err == nil {
			sess.Title = title
		}
	}

	turn, err := r.store.StartTurn(sessionID, sess.Model, sess.Effort)
	if err != nil {
		release()
		return nil, err
	}
	if _, err := r.store.AddMessage(sessionID, turn.ID, store.RoleUser, prompt); err != nil {
		release()
		return nil, err
	}
	if err := r.store.SetSessionStatus(sessionID, store.StatusRunning); err != nil {
		release()
		return nil, err
	}

	events, err := provider.Run(ctx, agent.TurnRequest{
		WorkDir:           sess.ProjectPath,
		Prompt:            prompt,
		Model:             sess.Model,
		Effort:            sess.Effort,
		SessionID:         sess.ID,
		ProviderSessionID: sess.ProviderSessionID,
		Permission:        agent.Permission(sess.Permission),
	})
	if err != nil {
		turn.Status, turn.Error = "error", err.Error()
		r.store.FinishTurn(turn)
		r.store.AddMessage(sessionID, turn.ID, store.RoleError, err.Error())
		r.store.SetSessionStatus(sessionID, store.StatusError)
		release()
		return nil, err
	}

	go r.consume(sess, turn, events, release)
	return turn, nil
}

// Stop cancels the running turn, which kills the CLI's process group.
func (r *Runner) Stop(sessionID string) error {
	r.mu.Lock()
	cancel, ok := r.active[sessionID]
	r.mu.Unlock()
	if !ok {
		return ErrNotRunning
	}
	cancel()
	return nil
}

// StopAll cancels every running turn, for shutdown.
func (r *Runner) StopAll() {
	r.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(r.active))
	for _, c := range r.active {
		cancels = append(cancels, c)
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func (r *Runner) consume(sess *store.Session, turn *store.Turn, events <-chan agent.Event, release func()) {
	defer release()

	var text strings.Builder
	var failure string

	// flushText writes the assistant prose accumulated so far. Called before
	// each tool call so the transcript keeps its original ordering.
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		r.store.AddMessage(sess.ID, turn.ID, store.RoleAssistant, text.String())
		text.Reset()
	}

	for ev := range events {
		switch ev.Type {
		case agent.EventSessionStarted:
			if ev.ProviderSessionID != "" && ev.ProviderSessionID != sess.ProviderSessionID {
				r.store.SetProviderSessionID(sess.ID, ev.ProviderSessionID)
				sess.ProviderSessionID = ev.ProviderSessionID
			}
		case agent.EventText:
			text.WriteString(ev.Text)
		case agent.EventToolUse:
			flushText()
			if ev.Tool != nil {
				r.store.AddMessage(sess.ID, turn.ID, store.RoleTool,
					toolSummary(ev.Tool))
			}
		case agent.EventUsage:
			// Mid-turn usage reports the size of one prompt, not the turn's
			// totals, so it only moves the context gauge. The last one wins:
			// it describes the context as it stands when the turn ends.
			if ev.Usage != nil && ev.Usage.ContextTokens > 0 {
				turn.ContextTokens = ev.Usage.ContextTokens
			}
		case agent.EventDone:
			if ev.Usage != nil {
				turn.InputTokens = ev.Usage.InputTokens
				turn.OutputTokens = ev.Usage.OutputTokens
				turn.CacheReadTokens = ev.Usage.CacheReadTokens
				turn.CacheWriteTokens = ev.Usage.CacheWriteTokens
				turn.CostUSD = ev.Usage.CostUSD
			}
		case agent.EventError:
			failure = ev.Text
			r.store.AddMessage(sess.ID, turn.ID, store.RoleError, ev.Text)
		}
		r.hub.Publish(sess.ID, Event{SessionID: sess.ID, TurnID: turn.ID, Event: ev})
	}
	flushText()

	turn.Status = "ok"
	sessionStatus := store.StatusIdle
	if failure != "" {
		turn.Status, turn.Error = "error", failure
		sessionStatus = store.StatusError
	}
	r.store.FinishTurn(turn)
	r.store.SetSessionStatus(sess.ID, sessionStatus)

	stats, _ := r.store.SessionStats(sess.ID)
	done := Event{SessionID: sess.ID, TurnID: turn.ID,
		Event: agent.Event{Type: agent.EventDone}, Stats: stats}
	r.hub.Publish(sess.ID, done)
	r.hub.Publish(ProjectTopic(sess.ProjectID), done)
}

// toolSummary is a one-line record of a tool call for the transcript. The full
// input is not stored: it can be megabytes and the UI only shows a summary.
func toolSummary(t *agent.ToolEvent) string {
	input := strings.TrimSpace(t.Input)
	const max = 400
	if len(input) > max {
		input = input[:max] + "…"
	}
	return t.Name + " " + input
}

// titleFrom derives a session title from its first prompt.
func titleFrom(prompt string) string {
	title := strings.TrimSpace(strings.SplitN(prompt, "\n", 2)[0])
	const max = 80
	if len([]rune(title)) > max {
		title = string([]rune(title)[:max]) + "…"
	}
	if title == "" {
		title = "Untitled session"
	}
	return title
}
