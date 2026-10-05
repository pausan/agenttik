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
