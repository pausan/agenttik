package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func moveFixture(t *testing.T, s *Store, path, session string) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO projects(id,name,path,created_at,prompt) VALUES(1,'Project','` + path + `',123,'Standing prompt')`,
		`INSERT INTO schedules(id,project_id,prompt,provider,model,every,anchor_at,created_at,account_id) VALUES(1,1,'Scheduled prompt','fake','m','day',1,2,9)`,
		`INSERT INTO sessions(id,project_id,title,provider,model,created_at,updated_at,last_active_at,schedule_id,account_id,provider_session_id,done_at,summary) VALUES('` + session + `',1,'Task','fake','m',3,4,5,1,9,'old-thread',6,'Outcome')`,
		`INSERT INTO turns(id,session_id,model,started_at,ended_at,status,input_tokens,cost_usd) VALUES(1,'` + session + `','m',7,8,'ok',123,0.25)`,
		`INSERT INTO messages(id,session_id,turn_id,role,content,created_at) VALUES(1,'` + session + `',1,'user','Hello',7)`,
		`INSERT INTO schedule_runs(id,schedule_id,session_id,status,started_at,ended_at,count) VALUES(1,1,'` + session + `','done',7,8,3)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMoveProjectPreservesGraphAndRemapsIDs(t *testing.T) {
	source, target := testStore(t), testStore(t)
	moveFixture(t, source, "/source", "source-task")
	moveFixture(t, target, "/target", "target-task")
	attachment := strings.Repeat("a", 64) + ".png"
	image := "![Attached image](/api/attachments/" + attachment + ")"
	if _, err := source.db.Exec(`UPDATE messages SET content = ?`, image); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(source.Dir(), "attachments", attachment)
	if err := os.MkdirAll(filepath.Dir(imagePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	thread := "direct-00000000-0000-4000-8000-000000000001"
	if _, err := source.db.Exec(`INSERT INTO sessions(id,project_id,provider,model,created_at,updated_at,last_active_at,provider_session_id) VALUES('api-task',1,'openai','m',1,1,1,?)`, thread); err != nil {
		t.Fatal(err)
	}
	history := filepath.Join(source.Dir(), "api-providers", "openai", "sessions", thread+".json")
	if err := os.MkdirAll(filepath.Dir(history), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(history, []byte(`{"messages":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := source.MoveProjectTo(target, 1)
	if err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("id = %d", id)
	}
	if _, err := source.GetProject(1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source remains: %v", err)
	}
	p, err := target.GetProject(id)
	if err != nil || p.Prompt != "Standing prompt" || p.CreatedAt != 123 || p.Path != "/source" {
		t.Fatalf("project: %+v %v", p, err)
	}
	task, err := target.GetSession("source-task")
	if err != nil || task.ProjectID != id || task.ScheduleID != 2 || task.AccountID != 0 || task.ProviderSessionID != "" || task.Summary != "Outcome" || task.DoneAt != 6 {
		t.Fatalf("task: %+v %v", task, err)
	}
	var turnID, tokens, runSchedule, count, paused int64
	var content string
	if err := target.db.QueryRow(`SELECT turn_id,content FROM messages WHERE session_id='source-task'`).Scan(&turnID, &content); err != nil {
		t.Fatal(err)
	}
	if turnID != 2 || content != image {
		t.Fatalf("message: %d %s", turnID, content)
	}
	if err := target.db.QueryRow(`SELECT input_tokens FROM turns WHERE id=?`, turnID).Scan(&tokens); err != nil || tokens != 123 {
		t.Fatalf("usage: %d %v", tokens, err)
	}
	if err := target.db.QueryRow(`SELECT schedule_id,count FROM schedule_runs WHERE session_id='source-task'`).Scan(&runSchedule, &count); err != nil || runSchedule != 2 || count != 3 {
		t.Fatalf("run: %d %d %v", runSchedule, count, err)
	}
	if err := target.db.QueryRow(`SELECT paused FROM schedules WHERE id=2`).Scan(&paused); err != nil || paused != 1 {
		t.Fatalf("paused: %d %v", paused, err)
	}
	original, err := target.GetSession("target-task")
	if err != nil || original.AccountID != 9 || original.ProjectID != 1 {
		t.Fatalf("destination changed: %+v %v", original, err)
	}
	apiTask, err := target.GetSession("api-task")
	if err != nil {
		t.Fatal(err)
	}
	if apiTask.ProviderSessionID == thread {
		t.Fatal("API file name must be unique at destination")
	}
	if data, err := os.ReadFile(filepath.Join(target.Dir(), "api-providers", "openai", "sessions", apiTask.ProviderSessionID+".json")); err != nil || string(data) != `{"messages":[]}` {
		t.Fatalf("history: %s %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(target.Dir(), "attachments", attachment)); err != nil || string(data) != "image bytes" {
		t.Fatalf("image: %s %v", data, err)
	}
	// A return move must work even though the old managed files still exist.
	if _, err = target.MoveProjectTo(source, id); err != nil {
		t.Fatalf("move back: %v", err)
	}
}

func TestMoveProjectFailureLeavesBothProfilesIntact(t *testing.T) {
	for _, scenario := range []string{"duplicate", "running", "queued", "schedule running", "orchestrator", "missing image", "session collision"} {
		t.Run(scenario, func(t *testing.T) {
			source, target := testStore(t), testStore(t)
			moveFixture(t, source, "/source", "task")
			moveFixture(t, target, "/target", "other")
			var query string
			switch scenario {
			case "duplicate":
				query = `UPDATE projects SET path='/target'`
			case "running":
				query = `UPDATE sessions SET status='running'`
			case "queued":
				query = `INSERT INTO queued_messages(session_id,prompt,created_at) VALUES('task','queued',1)`
			case "schedule running":
				query = `UPDATE schedule_runs SET status='running'`
			case "orchestrator":
				query = `UPDATE projects SET kind='orchestrator'`
			case "missing image":
				query = `UPDATE messages SET content='/api/attachments/` + strings.Repeat("a", 64) + `.png'`
			case "session collision":
				query = `INSERT INTO sessions(id,project_id,provider,model,created_at,updated_at,last_active_at) VALUES('other',1,'fake','m',1,1,1)`
			}
			if _, err := source.db.Exec(query); err != nil {
				t.Fatal(err)
			}
			if _, err := source.MoveProjectTo(target, 1); err == nil {
				t.Fatal("accepted invalid move")
			}
			if _, err := source.GetSession("task"); err != nil {
				t.Fatal("source lost", err)
			}
			if _, err := target.GetSession("task"); !errors.Is(err, ErrNotFound) {
				t.Fatal("partial destination", err)
			}
			for _, s := range []*Store{source, target} {
				var n int
				if err := s.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil || n != 1 {
					t.Fatalf("projects: %d %v", n, err)
				}
			}
		})
	}
}

func TestMoveProjectCleansCopiedFilesOnRollback(t *testing.T) {
	source, target := testStore(t), testStore(t)
	moveFixture(t, source, "/source", "task")
	name := strings.Repeat("b", 64) + ".png"
	if _, err := source.db.Exec(`UPDATE messages SET content = ?`, "/api/attachments/"+name); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(source.Dir(), "attachments")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.db.Exec(`CREATE TRIGGER prevent_move BEFORE DELETE ON projects BEGIN SELECT RAISE(ABORT,'injected delete failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := source.MoveProjectTo(target, 1); err == nil || !strings.Contains(err.Error(), "injected delete failure") {
		t.Fatalf("failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target.Dir(), "attachments", name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file remains: %v", err)
	}
	if _, err := source.GetSession("task"); err != nil {
		t.Fatal(err)
	}
	if _, err := target.GetProject(1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partial project remains: %v", err)
	}
}
