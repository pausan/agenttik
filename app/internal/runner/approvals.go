package runner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// ErrApprovalGone says the approval no longer waits: another window answered
// it first, agenttik answered it on time running out, or its turn ended.
var ErrApprovalGone = errors.New("approval already answered or no longer pending")

// PendingApproval is a request a running turn waits on. Pending approvals live
// only as long as their turn's process, so they are kept in memory, not in the
// store. See 082-tool-approvals.md.
type PendingApproval struct {
	SessionID string          `json:"session_id"`
	ProjectID int64           `json:"project_id"`
	TurnID    int64           `json:"turn_id"`
	CreatedAt int64           `json:"created_at"`
	Approval  *agent.Approval `json:"approval"`

	// timer answers the request at deadline when the settings say not to wait
	// for ever. Nil waits for the user.
	timer    *time.Timer
	deadline time.Time
}

// Approvals lists every pending approval, oldest first. A window reads it when
// its stream opens, so one that connects or reconnects after a request was
// published still shows it, with the time it has left as of now.
func (r *Runner) Approvals() []PendingApproval {
	r.mu.Lock()
	out := make([]PendingApproval, 0, len(r.approvals))
	for _, p := range r.approvals {
		pending := *p
		if p.timer != nil {
			// A copy: the published approval may be being serialized elsewhere.
			a := *p.Approval
			a.ExpiresIn = max(1, time.Until(p.deadline).Milliseconds())
			pending.Approval = &a
		}
		out = append(out, pending)
	}
	r.mu.Unlock()
	slices.SortFunc(out, func(a, b PendingApproval) int { return int(a.CreatedAt - b.CreatedAt) })
	return out
}

// ErrBadReply says a reply leaves a question unanswered or picks an option
// that was not offered. The request stays pending.
var ErrBadReply = errors.New("reply does not answer the questions")

// Answer allows, denies or answers a pending request. Taking it out of the map
// before answering is what makes the first answer win when several windows
// click at once, or a click races the timer; the rest get ErrApprovalGone. A
// reply that does not answer the questions is refused before that, so it uses
// nothing up.
func (r *Runner) Answer(sessionID, approvalID string, reply agent.Reply) error {
	r.mu.Lock()
	p, ok := r.approvals[approvalID]
	if !ok || p.SessionID != sessionID {
		r.mu.Unlock()
		return ErrApprovalGone
	}
	if err := p.Approval.CheckReply(reply); err != nil {
		r.mu.Unlock()
		return fmt.Errorf("%w: %v", ErrBadReply, err)
	}
	r.take(approvalID)
	// The task choices are agenttik's to keep; the provider sees a plain allow.
	var alsoAllowed []*PendingApproval
	switch reply.Remember {
	case agent.RememberTaskCommand, agent.RememberTask, agent.RememberTaskAll:
		alsoAllowed = r.allow(sessionID, p.Approval, reply.Remember)
		reply.Remember = ""
	}
	r.mu.Unlock()
	if err := p.Approval.Answer(reply); err != nil {
		r.publishResolved(p, nil, false)
		return ErrApprovalGone
	}
	r.publishReply(p, reply, false)
	for _, other := range alsoAllowed {
		if err := other.Approval.Answer(agent.Reply{Allow: true}); err != nil {
			r.publishResolved(other, nil, false)
			continue
		}
		r.publishReply(other, agent.Reply{Allow: true}, false)
	}
	return nil
}

// Hold stops a request's countdown, so it waits for the user after all. A
// request already waiting is left as it is.
func (r *Runner) Hold(sessionID, approvalID string) error {
	r.mu.Lock()
	p, ok := r.approvals[approvalID]
	if !ok || p.SessionID != sessionID {
		r.mu.Unlock()
		return ErrApprovalGone
	}
	if p.timer == nil {
		r.mu.Unlock()
		return nil
	}
	p.timer.Stop()
	p.timer = nil
	// Replaced rather than changed: the old one may still be on its way to a
	// window.
	held := *p.Approval
	held.ExpiresIn = 0
	held.Held = true
	p.Approval = &held
	r.mu.Unlock()
	// Sent again as a request: windows replace the card they hold by its id.
	r.publishApproval(p, &held)
	return nil
}

// allowance is what the user allowed for the rest of one task: every tool,
// some tools, or shell commands that start a certain way, by tool.
type allowance struct {
	all      bool
	tools    map[string]bool
	commands map[string][]string // tool -> command prefixes
}

// covers reports whether a tool call is allowed already. Questions are
// always asked.
func (w *allowance) covers(a *agent.Approval) bool {
	if w == nil || len(a.Questions) > 0 {
		return false
	}
	if w.all || w.tools[a.Tool] {
		return true
	}
	for _, prefix := range w.commands[a.Tool] {
		if agent.CommandHasPrefix(a.Input, prefix) {
			return true
		}
	}
	return false
}

// allow records an allow for the rest of the session's task, as remember
// says: a's command prefix, a's tool, or every tool. It takes the session's
// other pending tool calls that the allow now covers and returns them for the
// caller to answer. The caller holds the lock.
func (r *Runner) allow(sessionID string, a *agent.Approval, remember string) []*PendingApproval {
	w := r.allowed[sessionID]
	if w == nil {
		w = &allowance{tools: map[string]bool{}, commands: map[string][]string{}}
		r.allowed[sessionID] = w
	}
	switch remember {
	case agent.RememberTaskAll:
		w.all = true
	case agent.RememberTask:
		w.tools[a.Tool] = true
	case agent.RememberTaskCommand:
		w.commands[a.Tool] = append(w.commands[a.Tool], a.Prefix)
	}
	var covered []*PendingApproval
	for id, p := range r.approvals {
		if p.SessionID == sessionID && w.covers(p.Approval) {
			covered = append(covered, p)
			r.take(id)
		}
	}
	return covered
}

// isAllowed reports whether the user already allowed this tool call for the
// rest of its task.
func (r *Runner) isAllowed(sessionID string, a *agent.Approval) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.allowed[sessionID].covers(a)
}

// runMode is how much the project asks before its agents run tools. A store
// that cannot be read keeps the default, automatic.
func (r *Runner) runMode(projectID int64) string {
	p, err := r.store.GetProject(projectID)
	if err != nil {
		return store.RunAuto
	}
	return p.RunMode
}

// approvalPolicy is what the settings say to do with a request nobody
// answers. A store that cannot be read keeps the default.
func (r *Runner) approvalPolicy() (string, time.Duration) {
	config, err := r.store.GetGeneralConfig()
	if err != nil {
		return store.ApprovalTimeout, store.DefaultApprovalTimeout * time.Second
	}
	return config.ApprovalMode, time.Duration(config.ApprovalTimeout) * time.Second
}

// askUser handles a request the provider has just made. It reports whether
// the request was answered at once, in which case nobody is shown it.
// Otherwise it is pending, counting down when the settings say so; the caller
// publishes it.
func (r *Runner) askUser(sess *store.Session, turn *store.Turn, a *agent.Approval) bool {
	a.OfferTaskChoices()
	p := &PendingApproval{SessionID: sess.ID, ProjectID: sess.ProjectID,
		TurnID: turn.ID, CreatedAt: time.Now().UnixMilli(), Approval: a}
	if r.isAllowed(sess.ID, a) {
		reply := agent.Reply{Allow: true}
		if err := a.Answer(reply); err == nil {
			r.publishReply(p, reply, false)
		} else {
			r.publishResolved(p, nil, false)
		}
		return true
	}
	mode, wait := r.approvalPolicy()
	// A guarded or manual project asked to be asked: nobody allows a tool
	// call for the user. Questions still follow the settings.
	if len(a.Questions) == 0 && r.runMode(sess.ProjectID) != store.RunAuto {
		mode = store.ApprovalWait
	}
	if mode == store.ApprovalImmediate {
		reply := a.AutoReply()
		if err := a.Answer(reply); err == nil {
			r.publishReply(p, reply, true)
		} else {
			r.publishResolved(p, nil, false)
		}
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if mode == store.ApprovalTimeout && wait > 0 {
		a.ExpiresIn = wait.Milliseconds()
		p.deadline = time.Now().Add(wait)
		id := a.ID
		p.timer = time.AfterFunc(wait, func() { r.autoAnswer(id) })
	}
	r.approvals[a.ID] = p
	return false
}

// autoAnswer answers a request whose time ran out, as AutoReply says. A
// request answered in the meantime is gone, and nothing happens.
func (r *Runner) autoAnswer(id string) {
	r.mu.Lock()
	p, ok := r.approvals[id]
	if ok {
		r.take(id)
	}
	r.mu.Unlock()
	if !ok {
		return
	}
	reply := p.Approval.AutoReply()
	if err := p.Approval.Answer(reply); err != nil {
		r.publishResolved(p, nil, false)
		return
	}
	r.publishReply(p, reply, true)
}

// take removes a pending request and stops its countdown. The caller holds
// the lock.
func (r *Runner) take(id string) {
	if p, ok := r.approvals[id]; ok && p.timer != nil {
		p.timer.Stop()
	}
	delete(r.approvals, id)
}

// withdrawApproval drops an approval the provider stopped waiting on. It
// reports whether it was still pending, so a request answered a moment ago is
// not announced twice.
func (r *Runner) withdrawApproval(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.approvals[id]
	r.take(id)
	return ok
}

// dropApprovals withdraws whatever a finished turn left unanswered, so no
// window keeps offering buttons for a process that has exited.
func (r *Runner) dropApprovals(sessionID string) {
	r.mu.Lock()
	var dropped []*PendingApproval
	for id, p := range r.approvals {
		if p.SessionID == sessionID {
			dropped = append(dropped, p)
			r.take(id)
		}
	}
	r.mu.Unlock()
	for _, p := range dropped {
		r.publishResolved(p, nil, false)
	}
}

// publishApproval and publishResolved tell every window about a request. They
// go to the project topic as well as the session's, because the sidebar marks
// a task that waits on the user whether or not its tab is open.
func (r *Runner) publishApproval(p *PendingApproval, a *agent.Approval) {
	ev := Event{SessionID: p.SessionID, ProjectID: p.ProjectID, TurnID: p.TurnID,
		Event: agent.Event{Type: agent.EventApproval, Approval: a}}
	r.hub.Publish(p.SessionID, ev)
	r.hub.Publish(ProjectTopic(p.ProjectID), ev)
}

func (r *Runner) publishResolved(p *PendingApproval, allowed *bool, auto bool) {
	var message *store.Message
	if len(p.Approval.Questions) > 0 {
		var lines []string
		for _, q := range p.Approval.Questions {
			lines = append(lines, q.Question+"\nClosed without an accepted answer")
		}
		message, _ = r.store.AddMessage(p.SessionID, p.TurnID, store.RoleQuestion, strings.Join(lines, "\n\n"))
	}
	r.publishResolution(p, allowed, auto, message)
}

// Keep the question and its answer together after the pending card disappears.
// Secret answers go to the provider, but never to the store or other windows.
func (r *Runner) publishReply(p *PendingApproval, reply agent.Reply, auto bool) {
	var message *store.Message
	if len(p.Approval.Questions) > 0 {
		var lines []string
		for _, q := range p.Approval.Questions {
			answer := "Skipped"
			if reply.Allow {
				answer = strings.Join(reply.Answers[q.ID], ", ")
				if q.Secret {
					answer = "[hidden]"
				}
			}
			lines = append(lines, q.Question+"\n"+answer)
		}
		role := store.RoleAnswer
		if auto {
			role = store.RoleAutomaticAnswer
		}
		message, _ = r.store.AddMessage(p.SessionID, p.TurnID, role, strings.Join(lines, "\n\n"))
	}
	r.publishResolution(p, &reply.Allow, auto, message)
}

func (r *Runner) publishResolution(p *PendingApproval, allowed *bool, auto bool, message *store.Message) {
	ev := Event{SessionID: p.SessionID, ProjectID: p.ProjectID, TurnID: p.TurnID,
		Message: message,
		Event: agent.Event{Type: agent.EventApprovalResolved,
			Approval: &agent.Approval{ID: p.Approval.ID, Allowed: allowed, Auto: auto}}}
	r.hub.Publish(p.SessionID, ev)
	r.hub.Publish(ProjectTopic(p.ProjectID), ev)
}
