package store

import (
	"encoding/json"
	"testing"
)

func TestRestartStopsSavedBackgroundTasks(t *testing.T) {
	s := testStore(t)
	project, err := s.CreateProject("project", "/tmp/project")
	must(t, err)
	must(t, s.CreateSession(&Session{ID: "task", ProjectID: project.ID, Provider: "fake", Model: "fake", Status: StatusRunning}))
	turn, err := s.StartTurn("task", "fake", "")
	must(t, err)
	started, err := s.AddMessage("task", turn.ID, RoleBackground, `{"id":"b1","status":"running","started_at":10}`)
	must(t, err)
	finished, err := s.AddMessage("task", turn.ID, RoleBackground, `{"id":"b2","status":"completed","started_at":10,"ended_at":20}`)
	must(t, err)
	must(t, s.ResetRunningSessions())
	messages, err := s.ListMessages("task")
	must(t, err)
	if len(messages) != 2 || messages[0].ID != started.ID || messages[0].CreatedAt != started.CreatedAt ||
		messages[1].Content != finished.Content {
		t.Fatalf("messages after restart = %+v", messages)
	}
	var task struct {
		Status  string `json:"status"`
		EndedAt int64  `json:"ended_at"`
		Summary string `json:"summary"`
	}
	must(t, json.Unmarshal([]byte(messages[0].Content), &task))
	if task.Status != "stopped" || task.EndedAt == 0 || task.Summary != "turn interrupted" {
		t.Fatalf("task after restart = %+v", task)
	}
	// Another startup leaves the recorded stop time alone.
	must(t, s.ResetRunningSessions())
	again, err := s.GetMessage(started.ID)
	must(t, err)
	if again.Content != messages[0].Content {
		t.Errorf("second restart changed the task: %s", again.Content)
	}
}

// A prompt accepted into a running turn is stored without a turn, joins the
// turn when the agent starts on it, and moves below a reply that was stored
// after it. One that is already last keeps its id.
func TestMoveMessageToEnd(t *testing.T) {
	s := testStore(t)
	project, err := s.CreateProject("project", "/tmp/project")
	must(t, err)
	must(t, s.CreateSession(&Session{ID: "task", ProjectID: project.ID, Provider: "fake", Model: "fake"}))
	turn, err := s.StartTurn("task", "fake", "")
	must(t, err)
	prompt, err := s.AddMessage("task", 0, RoleUser, "status?")
	must(t, err)
	if got, _ := s.GetMessage(prompt.ID); got.TurnID != 0 {
		t.Fatalf("accepted prompt has turn %d", got.TurnID)
	}
	moved, replaced, err := s.MoveMessageToEnd(prompt.ID, turn.ID)
	must(t, err)
	if moved.ID != prompt.ID || replaced != 0 || moved.TurnID != turn.ID {
		t.Fatalf("last message moved: %+v replaced %d", moved, replaced)
	}
	if _, _, err := s.MoveMessageToEnd(prompt.ID, 0); err != ErrNotFound {
		t.Fatalf("moving a prompt a turn has: %v", err)
	}

	late, err := s.AddMessage("task", 0, RoleUser, "and now?")
	must(t, err)
	_, err = s.AddMessage("task", turn.ID, RoleAssistant, "Game A built.")
	must(t, err)
	moved, replaced, err = s.MoveMessageToEnd(late.ID, turn.ID)
	must(t, err)
	if replaced != late.ID || moved.ID <= late.ID || moved.CreatedAt != late.CreatedAt || moved.Content != "and now?" {
		t.Fatalf("moved %+v replaced %d", moved, replaced)
	}
	messages, err := s.ListMessages("task")
	must(t, err)
	var order []string
	for _, m := range messages {
		order = append(order, m.Content)
	}
	if len(order) != 3 || order[1] != "Game A built." || order[2] != "and now?" {
		t.Fatalf("order = %q", order)
	}
	if _, _, err := s.MoveMessageToEnd(late.ID, turn.ID); err != ErrNotFound {
		t.Fatalf("moving a replaced id: %v", err)
	}
}

// A prompt the agent never started on goes back to the queue with the choice
// it is to run on, and leaves the transcript.
func TestMessageToQueue(t *testing.T) {
	s := testStore(t)
	project, err := s.CreateProject("project", "/tmp/project")
	must(t, err)
	must(t, s.CreateSession(&Session{ID: "task", ProjectID: project.ID, Provider: "fake", Model: "fake"}))
	prompt, err := s.AddMessage("task", 0, RoleUser, "deploy when ready")
	must(t, err)
	must(t, s.MessageToQueue(prompt.ID, "fake", 0, "m2", "high"))
	if messages, _ := s.ListMessages("task"); len(messages) != 0 {
		t.Fatalf("messages = %+v", messages)
	}
	queued, err := s.ListQueuedMessages("task")
	must(t, err)
	if len(queued) != 1 || queued[0].Prompt != "deploy when ready" || queued[0].Model != "m2" || queued[0].Effort != "high" {
		t.Fatalf("queued = %+v", queued)
	}
	if err := s.MessageToQueue(prompt.ID, "fake", 0, "m2", "high"); err != ErrNotFound {
		t.Fatalf("requeueing it twice: %v", err)
	}

	// One that ran stays where it is.
	turn, err := s.StartTurn("task", "fake", "")
	must(t, err)
	ran, err := s.AddMessage("task", turn.ID, RoleUser, "build")
	must(t, err)
	if err := s.MessageToQueue(ran.ID, "fake", 0, "m2", "high"); err != ErrNotFound {
		t.Fatalf("requeueing a prompt that ran: %v", err)
	}
	if messages, _ := s.ListMessages("task"); len(messages) != 1 {
		t.Fatalf("messages = %+v", messages)
	}
	if queued, _ := s.ListQueuedMessages("task"); len(queued) != 1 {
		t.Fatalf("queued = %+v", queued)
	}
}

// A queued prompt handed to a running turn leaves the queue for the
// transcript in one step, keeping the time it was accepted. Only one caller
// gets a row.
func TestQueuedToMessage(t *testing.T) {
	s := testStore(t)
	project, err := s.CreateProject("project", "/tmp/project")
	must(t, err)
	must(t, s.CreateSession(&Session{ID: "task", ProjectID: project.ID, Provider: "fake", Model: "fake"}))
	queued, err := s.EnqueueMessage("task", "deploy when ready", "fake", 0, "m2", "high")
	must(t, err)
	m, err := s.QueuedToMessage(queued.ID)
	must(t, err)
	if m.Role != RoleUser || m.Content != "deploy when ready" || m.TurnID != 0 || m.CreatedAt != queued.CreatedAt {
		t.Fatalf("message = %+v", m)
	}
	if left, _ := s.ListQueuedMessages("task"); len(left) != 0 {
		t.Fatalf("queue = %+v", left)
	}
	if messages, _ := s.ListMessages("task"); len(messages) != 1 || messages[0].ID != m.ID {
		t.Fatalf("messages = %+v", messages)
	}
	if _, err := s.QueuedToMessage(queued.ID); err != ErrNotFound {
		t.Fatalf("taking it twice: %v", err)
	}
}
