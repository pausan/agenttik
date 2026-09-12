package store

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewItemPosition(t *testing.T) {
	for _, position := range []string{"top", "bottom"} {
		t.Run(position, func(t *testing.T) {
			s := testStore(t)
			cfg, err := s.GetGeneralConfig()
			must(t, err)
			if cfg.NewItemPosition != "top" {
				t.Fatalf("default = %q", cfg.NewItemPosition)
			}
			a, err := s.CreateProject("a", "/a")
			must(t, err)
			b, err := s.CreateProject("b", "/b")
			must(t, err)
			must(t, s.ReorderProjects([]int64{a.ID, b.ID}))
			for _, id := range []string{"a", "b"} {
				must(t, s.CreateSession(&Session{ID: id, ProjectID: a.ID, Provider: "fake"}))
			}
			must(t, s.ReorderSessions(a.ID, []string{"a", "b"}))
			must(t, s.SetGeneralConfig(GeneralConfig{NewItemPosition: position}))
			c, err := s.CreateProject("c", "/c")
			must(t, err)
			d, err := s.CreateProject("d", "/d")
			must(t, err)
			for _, id := range []string{"c", "d"} {
				must(t, s.CreateSession(&Session{ID: id, ProjectID: a.ID, Provider: "fake"}))
			}
			wantProjects := []int64{d.ID, c.ID, a.ID, b.ID}
			wantTasks := []string{"d", "c", "a", "b"}
			if position == "bottom" {
				wantProjects = []int64{a.ID, b.ID, c.ID, d.ID}
				wantTasks = []string{"a", "b", "c", "d"}
			}
			projects, err := s.ListProjects()
			must(t, err)
			var gotProjects []int64
			for _, p := range projects {
				gotProjects = append(gotProjects, p.ID)
				if p.ID == a.ID {
					var got []string
					for _, task := range p.RecentSessions {
						got = append(got, task.ID)
					}
					if !reflect.DeepEqual(got, wantTasks) {
						t.Fatalf("sidebar tasks = %v", got)
					}
				}
			}
			if !reflect.DeepEqual(gotProjects, wantProjects) {
				t.Fatalf("projects = %v", gotProjects)
			}
			tasks, err := s.ListSessions(SessionFilter{ProjectID: a.ID})
			must(t, err)
			if !reflect.DeepEqual(ids(tasks), wantTasks) {
				t.Fatalf("project tasks = %v", ids(tasks))
			}
		})
	}
}

func TestGeneralConfigPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	s, err := Open(path)
	must(t, err)
	must(t, s.SetGeneralConfig(GeneralConfig{NewItemPosition: "bottom"}))
	must(t, s.Close())
	s, err = Open(path)
	must(t, err)
	defer s.Close()
	cfg, err := s.GetGeneralConfig()
	must(t, err)
	if cfg.NewItemPosition != "bottom" {
		t.Fatalf("saved position = %q", cfg.NewItemPosition)
	}
}
