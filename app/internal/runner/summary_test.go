package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// smallProvider answers the isolated request agenttik makes *about* a task and
// records what it was asked, so a test can see both the summary that came back
// and the question that went out.
type smallProvider struct {
	reply string
	gate  chan struct{}          // if non-nil, the answer waits for it
	asked chan agent.TurnRequest // what the isolated requests carried
}

func newSmallProvider(reply string) *smallProvider {
	return &smallProvider{reply: reply, asked: make(chan agent.TurnRequest, 4)}
}

func (s *smallProvider) Name() string          { return "fake" }
func (s *smallProvider) DisplayName() string   { return "Fake" }
func (s *smallProvider) Models() []agent.Model { return []agent.Model{{ID: "m1", Label: "M1"}} }
func (s *smallProvider) Efforts() []string     { return []string{"high"} }
func (s *smallProvider) Available() error      { return nil }

func (s *smallProvider) SmallModel() (model, effort string) { return "m1", "low" }

func (s *smallProvider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 2)
	if req.Isolated {
		s.asked <- req
	}
	go func() {
		defer close(ch)
		if s.gate != nil {
			select {
			case <-s.gate:
			case <-ctx.Done():
				return
			}
		}
		ch <- agent.Event{Type: agent.EventText, Text: s.reply}
		ch <- agent.Event{Type: agent.EventDone}
	}()
	return ch, nil
}

// finished puts a task in the state a summary is written from: something the
// agent said, and the archive flag set.
func finished(t *testing.T, st *store.Store, sess *store.Session, reply string) {
	t.Helper()
	if reply != "" {
		if _, err := st.AddMessage(sess.ID, 0, store.RoleAssistant, reply); err != nil {
			t.Fatalf("add reply: %v", err)
		}
	}
	if err := st.SetSessionDone(sess.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
}

func TestArchivingSummarizesFromTheLastReply(t *testing.T) {
	sp := newSmallProvider("Removed the sleep from the queue scheduler test")
	r, st, sess := setup(t, sp)
	// Prose is flushed before each tool call, so the newest assistant row is
	// the closing text of the turn and not the whole of it.
	finished(t, st, sess, "First I looked at the scheduler.")
	if _, err := st.AddMessage(sess.ID, 0, store.RoleAssistant, "Done — the sleep is gone and the test passes."); err != nil {
		t.Fatalf("add reply: %v", err)
	}

	r.SummarizeTask(sess.ID)
	waitFor(t, func() bool {
		got, err := st.GetSession(sess.ID)
		return err == nil && got.Summary == "Removed the sleep from the queue scheduler test"
	}, "the summary to be written")

	req := <-sp.asked
	if !strings.Contains(req.Prompt, "Done — the sleep is gone") {
		t.Errorf("the question did not carry the last reply: %q", req.Prompt)
	}
	if strings.Contains(req.Prompt, "First I looked at") {
		t.Error("the question carried an earlier reply as well as the last one")
	}
	// The same rules a title request runs under: no session to join, no
	// project to touch, and nothing it may write to.
	if !req.Isolated || req.Permission != agent.PermissionPlan || req.SessionID != "" {
		t.Errorf("summary request was not isolated and read-only: %+v", req)
	}
}

func TestSummaryOfATaskThatSaidNothingIsBlank(t *testing.T) {
	sp := newSmallProvider("something")
	r, st, sess := setup(t, sp)
	finished(t, st, sess, "") // archived without ever running

	r.SummarizeTask(sess.ID)
	select {
	case req := <-sp.asked:
		t.Fatalf("asked about a task with no reply: %+v", req)
	case <-time.After(200 * time.Millisecond):
	}
	got, err := st.GetSession(sess.ID)
	if err != nil || got.Summary != "" {
		t.Errorf("summary = %q, want blank", got.Summary)
	}
}

// TestSummaryIsDroppedWhenTheTaskComesBack holds the answer until the task has
// been restored. A task someone pulled back out of the archive is not finished,
// so the line describing how it finished is not written.
func TestSummaryIsDroppedWhenTheTaskComesBack(t *testing.T) {
	sp := newSmallProvider("Fixed it")
	sp.gate = make(chan struct{})
	r, st, sess := setup(t, sp)
	finished(t, st, sess, "All done.")

	r.SummarizeTask(sess.ID)
	<-sp.asked // the request is in flight
	if err := st.SetSessionDone(sess.ID, false); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	close(sp.gate)

	waitFor(t, func() bool {
		got, err := st.GetSession(sess.ID)
		return err == nil && got.DoneAt == 0
	}, "the task to be back in the open list")
	got, _ := st.GetSession(sess.ID)
	if got.Summary != "" {
		t.Errorf("summary = %q, want blank: the task was restored before the answer landed", got.Summary)
	}
}

func TestSummaryFromResponse(t *testing.T) {
	cases := []struct {
		name     string
		response string
		want     string
	}{
		{"a plain sentence", "Fixed the flaky queue scheduler test", "Fixed the flaky queue scheduler test"},
		{"only the first line", "Added the summary column\n\nIt also updates the spec.", "Added the summary column"},
		{"a prefix the rules asked it not to write", "Summary: Renamed the provider interface", "Renamed the provider interface"},
		{"quotes around the whole line", "\"Dropped the dead sessions pane\"", "Dropped the dead sessions pane"},
		{"nothing at all", "   \n  ", ""},
		{"longer than a row keeps", strings.Repeat("x", summaryMax+20), strings.Repeat("x", summaryMax) + "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := summaryFromResponse(c.response); got != c.want {
				t.Errorf("summaryFromResponse(%q) = %q, want %q", c.response, got, c.want)
			}
		})
	}
}
