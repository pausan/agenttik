package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

const capacityFailure = "Selected model is at capacity. Please try a different model."

func TestRetryAt(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name     string
		retries  int64
		capacity bool
		reset    time.Duration
		want     time.Duration
	}{
		{"first outage", 0, false, 0, time.Minute},
		{"repeated outage", 1, false, 0, 2 * time.Minute},
		{"capped outage", 100, false, 0, 15 * time.Minute},
		{"first capacity refusal", 0, true, 0, 30 * time.Second},
		{"repeated capacity refusal", 100, true, 0, 30 * time.Second},
		{"allowance reset", 0, false, time.Hour, time.Hour},
		{"capacity reset", 100, true, time.Hour, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryAt(now, now.Add(tc.reset), tc.retries, tc.capacity); !got.Equal(now.Add(tc.want)) {
				t.Fatalf("retry in %v, want %v", got.Sub(now), tc.want)
			}
		})
	}
}

// capacityProvider refuses two attempts, then recovers on the same model.
type capacityProvider struct {
	fakeProvider
	requests  chan agent.TurnRequest
	remaining int
	startup   bool
	partial   bool
}

func (p *capacityProvider) Run(_ context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	p.requests <- req
	fail := p.remaining > 0
	if fail {
		p.remaining--
		if p.startup {
			return nil, errors.New(capacityFailure)
		}
	}
	out := make(chan agent.Event, 4)
	out <- agent.Event{Type: agent.EventSessionStarted, ProviderSessionID: "provider-thread"}
	if fail {
		if p.partial {
			out <- agent.Event{Type: agent.EventText, Text: "work before capacity refusal"}
		}
		out <- agent.Event{Type: agent.EventError, Text: capacityFailure}
	} else {
		out <- agent.Event{Type: agent.EventText, Text: "resumed"}
		out <- agent.Event{Type: agent.EventDone}
	}
	close(out)
	return out, nil
}

func TestCapacityWaitAndRecovery(t *testing.T) {
	for _, mode := range []string{"startup", "stream", "partial"} {
		t.Run(mode, func(t *testing.T) {
			fp := &capacityProvider{requests: make(chan agent.TurnRequest, 4), remaining: 2,
				startup: mode == "startup", partial: mode == "partial"}
			r, st, sess := setup(t, fp)
			t.Cleanup(r.Shutdown)
			started := time.Now()
			_, err := r.Send(sess.ID, "original prompt")
			if err != nil && !errors.Is(err, ErrProviderAway) {
				t.Fatal(err)
			}
			for attempt := int64(1); attempt <= 2; attempt++ {
				waitFor(t, func() bool { return !r.Running(sess.ID) }, "capacity refusal")
				// Join the completion's scheduler before changing its queue.
				r.schedule(sess.ProjectID)
				updated, err := st.GetSession(sess.ID)
				if err != nil || updated.Status != store.StatusWaiting {
					t.Fatalf("session = %+v, %v", updated, err)
				}
				queue, err := st.ListQueuedMessages(sess.ID)
				if err != nil || len(queue) == 0 {
					t.Fatalf("queue = %+v, %v", queue, err)
				}
				q := queue[0]
				// Expiring the persisted deadline below also counts as a requeue.
				if q.Prompt != "original prompt" || q.Model != sess.Model || q.Effort != sess.Effort ||
					q.RetryError != capacityFailure || q.RetryCount != 2*attempt-1 {
					t.Fatalf("held prompt = %+v", q)
				}
				if q.RetryAt < started.Add(30*time.Second).UnixMilli() ||
					q.RetryAt > time.Now().Add(30*time.Second).UnixMilli() {
					t.Fatalf("capacity deadline = %v", time.UnixMilli(q.RetryAt))
				}
				request := <-fp.requests
				if request.Prompt != "original prompt" || request.Model != sess.Model || request.Effort != sess.Effort {
					t.Fatalf("request = %+v", request)
				}
				if attempt == 1 {
					if _, err := r.Enqueue(sess.ID, "later prompt"); err != nil {
						t.Fatal(err)
					}
				}
				select {
				case req := <-fp.requests:
					t.Fatalf("ran while waiting: %+v", req)
				default:
				}
				turns, err := st.ListTurns(sess.ID)
				if err != nil {
					t.Fatal(err)
				}
				wantTurns := 0
				if fp.partial {
					wantTurns = int(attempt)
				}
				if len(turns) != wantTurns {
					t.Fatalf("turns = %+v; want %d", turns, wantTurns)
				}
				// Simulate the persisted deadline becoming due, then invoke the
				// same scheduler used by the retry clock. No user send is needed.
				if err := st.RemoveQueuedMessage(q.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := st.RequeueMessage(q, time.Now().Add(-time.Second).UnixMilli(), capacityFailure); err != nil {
					t.Fatal(err)
				}
				r.schedule(sess.ProjectID)
			}
			waitFor(t, func() bool {
				count, _ := st.QueueCount(sess.ID)
				return count == 0 && !r.Running(sess.ID)
			}, "recovered task and follow-up to finish")
			for _, prompt := range []string{"original prompt", "later prompt"} {
				req := <-fp.requests
				if req.Prompt != prompt || req.Model != sess.Model {
					t.Fatalf("recovery request = %+v, want %q", req, prompt)
				}
				if !fp.startup && req.ProviderSessionID != "provider-thread" {
					t.Fatalf("lost provider conversation: %+v", req)
				}
			}
			updated, err := st.GetSession(sess.ID)
			if err != nil || updated.Status != store.StatusIdle {
				t.Fatalf("recovered session = %+v, %v", updated, err)
			}
			messages, err := st.ListMessages(sess.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, msg := range messages {
				if msg.Role == store.RoleError && !fp.partial {
					t.Fatalf("empty refusal left an error: %+v", msg)
				}
			}
		})
	}
}
