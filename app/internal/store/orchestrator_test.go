package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pausan/agenttik/app/internal/orchestrator"
)

func TestOrchestratorLifecycle(t *testing.T) {
	s := testStore(t)
	cfg, err := s.GetOrchestratorConfig()
	if err != nil || cfg.Enabled || cfg.ProjectID != 0 || cfg.Prompt != orchestrator.DefaultPrompt ||
		cfg.Path != filepath.Join(s.Dir(), "orchestrator") {
		t.Fatalf("initial config = %+v, %v", cfg, err)
	}
	ordinary, err := s.CreateProject("Orchestrator", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetOrchestratorEnabled(true); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.GetOrchestratorConfig()
	id := cfg.ProjectID
	if err := s.ReorderProjects([]int64{ordinary.ID, id}); err != nil {
		t.Fatal(err)
	}
	projects, err := s.ListProjects()
	if err != nil || len(projects) != 2 || projects[0].ID != id || projects[0].Kind != "orchestrator" || projects[1].Kind != "" {
		t.Fatalf("ordered projects = %+v, %v", projects, err)
	}
	if err := s.CreateSession(&Session{ID: "orchestrator-task", ProjectID: id, Provider: "fake", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	customPath := t.TempDir()
	for _, err := range []error{s.SetProjectName(id, "Coordinator"), s.SetProjectPath(id, customPath), s.SetProjectPrompt(id, "Custom instructions")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetOrchestratorEnabled(false); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.GetOrchestratorConfig()
	if cfg.Enabled || cfg.ProjectID != id {
		t.Fatalf("disabled config = %+v", cfg)
	}
	if _, err := s.GetSession("orchestrator-task"); err != nil {
		t.Fatal("disabling lost task:", err)
	}
	if err := s.SetOrchestratorEnabled(true); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.GetOrchestratorConfig()
	if !cfg.Enabled || cfg.ProjectID != id || cfg.Prompt != "Custom instructions" {
		t.Fatalf("restored config = %+v", cfg)
	}
	if err := s.DeleteProject(id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession("orchestrator-task"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted task = %v", err)
	}
	cfg, _ = s.GetOrchestratorConfig()
	if cfg.Enabled || cfg.ProjectID != 0 || cfg.Name != "Coordinator" || cfg.Path != customPath || cfg.Prompt != "Custom instructions" {
		t.Fatalf("deleted config = %+v", cfg)
	}
	// Reopen the database: both the disabled state and saved options are durable.
	reopened, err := Open(filepath.Join(s.Dir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.SetOrchestratorEnabled(true); err != nil {
		t.Fatal(err)
	}
	cfg, _ = reopened.GetOrchestratorConfig()
	project, err := reopened.GetProject(cfg.ProjectID)
	if err != nil || project.Name != "Coordinator" || project.Path != customPath || project.Prompt != "Custom instructions" {
		t.Fatalf("recreated project = %+v, %v", project, err)
	}
	tasks, err := reopened.ListSessions(SessionFilter{ProjectID: project.ID})
	if err != nil || len(tasks) != 0 {
		t.Fatalf("recreated tasks = %+v, %v", tasks, err)
	}
}

func TestOrchestratorEmptyAndResetPrompt(t *testing.T) {
	s := testStore(t)
	if err := s.SetOrchestratorEnabled(true); err != nil {
		t.Fatal(err)
	}
	cfg, _ := s.GetOrchestratorConfig()
	if err := s.SetProjectPrompt(cfg.ProjectID, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(cfg.ProjectID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOrchestratorEnabled(true); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.GetOrchestratorConfig()
	if cfg.Prompt != "" {
		t.Fatal("empty prompt replaced by default")
	}
	if err := s.ResetOrchestratorPrompt(); err != nil {
		t.Fatal(err)
	}
	project, _ := s.GetProject(cfg.ProjectID)
	if project.Prompt != orchestrator.DefaultPrompt {
		t.Fatal("reset did not reach existing project")
	}
	if err := s.DeleteProject(cfg.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE orchestrator_config SET prompt = 'old build prompt'`); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetOrchestratorPrompt(); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.GetOrchestratorConfig()
	if cfg.Enabled || cfg.ProjectID != 0 || cfg.Prompt != orchestrator.DefaultPrompt {
		t.Fatalf("reset after deletion = %+v", cfg)
	}
}

func TestConcurrentOrchestratorEnableAndFolderCollision(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.SetOrchestratorEnabled(true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	projects, err := s.ListProjects()
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects = %+v, %v", projects, err)
	}
	if err := s.DeleteProject(projects[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject("A different project", projects[0].Path); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOrchestratorEnabled(true); !errors.Is(err, ErrPathInUse) {
		t.Fatalf("folder collision = %v", err)
	}
	cfg, _ := s.GetOrchestratorConfig()
	if cfg.Enabled || cfg.ProjectID != 0 {
		t.Fatal("collision converted an ordinary project into the orchestrator")
	}
}
