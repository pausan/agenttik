package runner

import (
	"fmt"

	"github.com/pausan/agenttik/app/internal/store"
)

// Choice is what a turn runs on: the provider, the subscription, the model
// and the effort. The provider's process is started on it, so a prompt joins
// a running turn only on the same one. See 083-background-tasks.md.
type Choice struct {
	Provider  string `json:"provider"`
	AccountID int64  `json:"account_id"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
}

func (c Choice) String() string {
	return fmt.Sprintf("%s %s, effort %q, subscription %d", c.Provider, c.Model, c.Effort, c.AccountID)
}

// choiceOf is what a prompt is to run on: its queue row's saved choice, or
// for one sent straight to a session, the session's.
func choiceOf(queued store.QueuedMessage) Choice {
	return Choice{Provider: queued.Provider, AccountID: queued.AccountID, Model: queued.Model, Effort: queued.Effort}
}

// ChoiceError refuses a prompt the running turn takes prompts for, but runs
// on another choice than the prompt's. Nothing is consumed: the prompt is
// where it was.
type ChoiceError struct {
	Running Choice
	Wanted  Choice
}

func (e *ChoiceError) Error() string {
	return fmt.Sprintf("the running turn takes prompts only on %s, not %s", e.Running, e.Wanted)
}

// Mismatch is what a prompt does about a running turn that takes prompts on
// another choice than its own.
type Mismatch string

const (
	// Refuse leaves the prompt where it was, with a ChoiceError.
	Refuse Mismatch = ""
	// Join hands it to that turn anyway, to run on the turn's choice.
	Join Mismatch = "join"
	// Stop cancels that turn, background tasks and all, and runs the prompt
	// on its own choice once the turn has ended. Only Send now offers it: a
	// queued prompt waits for that in the queue.
	Stop Mismatch = "stop"
)

// RunningChoice is what the session's running turn runs on, nil when there
// is none.
func (r *Runner) RunningChoice(sessionID string) *Choice {
	r.mu.Lock()
	active, ok := r.active[sessionID]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	choice := active.open.choice
	return &choice
}
