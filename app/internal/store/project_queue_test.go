package store

import (
	"path/filepath"
	"testing"
)

func TestProjectQueuePauseSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paused.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject("paused", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if p.QueuePaused {
		t.Fatal("new project is paused")
	}
	if err := st.SetProjectQueuePaused(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err = st.GetProject(p.ID)
	if err != nil || !p.QueuePaused {
		t.Fatalf("reopened project = %+v, %v", p, err)
	}
}
