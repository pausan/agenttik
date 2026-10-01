package runner

import (
	"errors"
	"slices"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// ErrApprovalGone says the approval no longer waits: another window answered
// it first, or its turn ended.
var ErrApprovalGone = errors.New("approval already answered or no longer pending")

// PendingApproval is a tool call a running turn waits on. Pending approvals
// live only as long as their turn's process, so they are kept in memory, not
// in the store. See 082-tool-approvals.md.
type PendingApproval struct {
	SessionID string          `json:"session_id"`
	ProjectID int64           `json:"project_id"`
	TurnID    int64           `json:"turn_id"`
	CreatedAt int64           `json:"created_at"`
	Approval  *agent.Approval `json:"approval"`
}

// Approvals lists every pending approval, oldest first. A window reads it when
// its stream opens, so one that connects or reconnects after a request was
// published still shows it.
func (r *Runner) Approvals() []PendingApproval {
	r.mu.Lock()
	out := make([]PendingApproval, 0, len(r.approvals))
	for _, p := range r.approvals {
		out = append(out, *p)
	}
	r.mu.Unlock()
	slices.SortFunc(out, func(a, b PendingApproval) int { return int(a.CreatedAt - b.CreatedAt) })
	return out
}

// Answer allows or denies a pending approval. Taking it out of the map before
// answering is what makes the first answer win when several windows click at
// once; the rest get ErrApprovalGone.
func (r *Runner) Answer(sessionID, approvalID string, allow bool) error {
	r.mu.Lock()
	p, ok := r.approvals[approvalID]
	if ok && p.SessionID == sessionID {
		delete(r.approvals, approvalID)
	}
	r.mu.Unlock()
	if !ok || p.SessionID != sessionID {
		return ErrApprovalGone
	}
	if err := p.Approval.Answer(allow); err != nil {
		r.publishResolved(p, nil)
		return ErrApprovalGone
	}
	r.publishResolved(p, &allow)
	return nil
}

func (r *Runner) addApproval(sess *store.Session, turn *store.Turn, a *agent.Approval) {
	r.mu.Lock()
	r.approvals[a.ID] = &PendingApproval{SessionID: sess.ID, ProjectID: sess.ProjectID,
		TurnID: turn.ID, CreatedAt: time.Now().UnixMilli(), Approval: a}
	r.mu.Unlock()
}

// withdrawApproval drops an approval the provider stopped waiting on. It
// reports whether it was still pending, so a request answered a moment ago is
// not announced twice.
func (r *Runner) withdrawApproval(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.approvals[id]
	delete(r.approvals, id)
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
			delete(r.approvals, id)
		}
	}
	r.mu.Unlock()
	for _, p := range dropped {
		r.publishResolved(p, nil)
	}
}

// publishResolved tells every window an approval is settled. It goes to the
// project topic as well as the session's, because the sidebar marks a task
// that waits on approval whether or not its tab is open.
func (r *Runner) publishResolved(p *PendingApproval, allowed *bool) {
	ev := Event{SessionID: p.SessionID, ProjectID: p.ProjectID, TurnID: p.TurnID,
		Event: agent.Event{Type: agent.EventApprovalResolved,
			Approval: &agent.Approval{ID: p.Approval.ID, Allowed: allowed}}}
	r.hub.Publish(p.SessionID, ev)
	r.hub.Publish(ProjectTopic(p.ProjectID), ev)
}
