package runner

import (
	"errors"
	"fmt"
	"slices"
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
	r.mu.Unlock()
	if err := p.Approval.Answer(reply); err != nil {
		r.publishResolved(p, nil, false)
		return ErrApprovalGone
	}
	r.publishResolved(p, &reply.Allow, false)
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
	p.Approval = &held
	r.mu.Unlock()
	// Sent again as a request: windows replace the card they hold by its id.
	r.publishApproval(p, &held)
	return nil
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
	mode, wait := r.approvalPolicy()
	if mode == store.ApprovalImmediate {
		a.Answer(a.AutoReply())
		return true
	}
	p := &PendingApproval{SessionID: sess.ID, ProjectID: sess.ProjectID,
		TurnID: turn.ID, CreatedAt: time.Now().UnixMilli(), Approval: a}
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
	r.publishResolved(p, &reply.Allow, true)
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
	ev := Event{SessionID: p.SessionID, ProjectID: p.ProjectID, TurnID: p.TurnID,
		Event: agent.Event{Type: agent.EventApprovalResolved,
			Approval: &agent.Approval{ID: p.Approval.ID, Allowed: allowed, Auto: auto}}}
	r.hub.Publish(p.SessionID, ev)
	r.hub.Publish(ProjectTopic(p.ProjectID), ev)
}
