// Retrying a prompt whose provider was away. See specs/045-provider-outage-retry.md.
package runner

import (
	"context"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// retryTick is how often held prompts are looked at. It matches the schedule
// clock: being due is one indexed query, and a minute's wait that is checked
// every quarter minute is a minute to the eye.
const retryTick = 15 * time.Second

// The wait between attempts. The first is a minute away — the request itself
// is the only honest test of whether a provider is back, and a failed one
// returns at once and costs no tokens — and each failure after it doubles the
// wait to a quarter-hour ceiling. A provider down for an afternoon is then
// tried a couple of dozen times rather than three hundred, and one that
// volunteers when its allowance resets is tried once, then.
const (
	retryFirst = time.Minute
	retryMax   = 15 * time.Minute
)

// retryAt is when a prompt that has just failed should be tried again. retries
// is how many attempts it had already cost, so 0 is the first failure.
//
// A reset the provider named beats the backoff: it knows when its allowance
// comes back and guessing shorter only burns attempts against a wall. It is
// still not honoured below the first delay, since a reset already upon us
// would otherwise retry straight into the same failure.
func retryAt(now, resetAt time.Time, retries int64) time.Time {
	delay := retryFirst
	for i := int64(0); i < retries && delay < retryMax; i++ {
		delay *= 2
	}
	if delay > retryMax {
		delay = retryMax
	}
	soonest := now.Add(retryFirst)
	if at := now.Add(delay); at.After(soonest) {
		soonest = at
	}
	if resetAt.After(soonest) {
		return resetAt
	}
	return soonest
}

// holdForOutage puts a prompt back in the queue when its turn failed because
// the provider was away, and reports whether it did. A failure that is the
// request's own fault is not one of these and is left to be reported.
//
// produced says whether the turn got anywhere first. An attempt that produced
// nothing is erased — otherwise a provider that is down for an afternoon
// leaves a prompt bubble and an error bubble per attempt in the transcript,
// and counts each one among the turns the session is said to have spent, for
// work that never happened. An attempt that did produce something is kept and
// closed as the failure it was: whatever it did to the project is real, and
// the retry reads in the transcript as the second try it is.
func (r *Runner) holdForOutage(sess *store.Session, queued store.QueuedMessage, turn *store.Turn, failure string, produced bool) bool {
	away, resetAt := agent.Outage(failure)
	if !away {
		return false
	}
	at := retryAt(time.Now(), resetAt, queued.RetryCount)
	if _, err := r.store.RequeueMessage(queued, at.UnixMilli(), failure); err != nil {
		// The prompt could not be put anywhere safe, so it is reported as the
		// failure it is rather than quietly dropped.
		return false
	}
	if produced {
		turn.Status, turn.Error = "error", failure
		_ = r.store.FinishTurn(turn)
	} else {
		_ = r.store.DiscardTurn(turn.ID)
	}
	_ = r.store.SetSessionStatus(sess.ID, store.StatusWaiting)
	return true
}

// RunQueueRetries starts held prompts whose wait is over, until ctx is
// cancelled. It is also what recovers an outage the app was restarted during:
// the wait is on the queue row rather than in this process, so a prompt that
// came due while it was down is picked up on the first tick, and one still
// waiting keeps waiting instead of retrying on the way up.
func (r *Runner) RunQueueRetries(ctx context.Context) {
	ticker := time.NewTicker(retryTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			projects, err := r.store.RetryProjects(now.UnixMilli())
			if err != nil {
				continue
			}
			// The ordinary scheduler: the prompt is ready, so it runs when the
			// project is free, behind nothing and ahead of nothing. Waiting on
			// a provider was never a reason to jump the queue.
			for _, projectID := range projects {
				go r.schedule(projectID, "")
			}
		}
	}
}
