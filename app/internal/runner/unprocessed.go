package runner

import (
	"errors"
	"slices"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
)

// ErrNotUnprocessed says a prompt sent or queued from the transcript is not
// one the agent left unprocessed: it ran, the running turn holds it, or it is
// not yours. See 083-background-tasks.md.
var ErrNotUnprocessed = errors.New("not a prompt the agent left unprocessed")

// SendUnprocessed sends one of a session's prompts the agent never processed,
// as Send sends a typed one: into the running turn when that turn takes
// prompts, otherwise as a turn of its own. The message is not written again:
// it moves to the end of the transcript, where a prompt sent now goes, and is
// the turn's from there. joined is the message as stored when the running
// turn holds it.
func (r *Runner) SendUnprocessed(sessionID string, messageID int64) (turn *store.Turn, joined *store.Message, err error) {
	message, err := r.unprocessed(sessionID, messageID)
	if err != nil {
		return nil, nil, err
	}
	return r.send(store.QueuedMessage{SessionID: sessionID, Prompt: message.Content,
		CreatedAt: time.Now().UnixMilli()}, messageID)
}

// QueueUnprocessed puts one of a session's prompts the agent never processed
// at the end of its queue, as Enqueue queues a typed one, with the session's
// current choice of model. It leaves the transcript, and comes back to it
// when it runs.
func (r *Runner) QueueUnprocessed(sessionID string, messageID int64) ([]store.QueuedMessage, error) {
	message, err := r.unprocessed(sessionID, messageID)
	if err != nil {
		return nil, err
	}
	sess, err := r.enqueue(sessionID, message.Content, messageID)
	if err != nil {
		return nil, err
	}
	r.schedule(sess.ProjectID)
	return r.store.ListQueuedMessages(sessionID)
}

// unprocessed is one of a session's prompts the agent never processed: yours,
// without a turn, and not held by the running turn. A task spawned from a
// thread starts with copies of the thread's messages, without turns too; they
// ran in the thread, and are older than the task.
func (r *Runner) unprocessed(sessionID string, messageID int64) (*store.Message, error) {
	sess, err := r.store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	message, err := r.store.GetMessage(messageID)
	if err != nil {
		return nil, err
	}
	if message.SessionID != sessionID {
		return nil, store.ErrNotFound
	}
	if message.Role != store.RoleUser || message.TurnID != 0 || message.CreatedAt < sess.CreatedAt ||
		slices.Contains(r.PendingFollowUps(sessionID), messageID) {
		return nil, ErrNotUnprocessed
	}
	return message, nil
}

// queueStored moves an unprocessed prompt to its session's queue under the
// running turn's lock, if there is a turn: followUp hands prompts to it under
// that lock, and one on its way there is not to be queued as well.
func (r *Runner) queueStored(sess *store.Session, id int64) error {
	r.mu.Lock()
	active, ok := r.active[sess.ID]
	r.mu.Unlock()
	if ok {
		active.open.mu.Lock()
		defer active.open.mu.Unlock()
		if slices.Contains(active.open.pending, id) {
			return ErrNotUnprocessed
		}
	}
	return r.store.MessageToQueue(id, sess.Provider, sess.AccountID, sess.Model, sess.Effort)
}
