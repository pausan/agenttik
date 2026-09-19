package store

import (
	"fmt"
	"testing"
)

func TestSessionPagingAndNewest(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("paging", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		v := &Session{ID: fmt.Sprint(i), ProjectID: p.ID, Title: fmt.Sprintf("task %d", i)}
		if err := s.CreateSession(v); err != nil {
			t.Fatal(err)
		}
		// Equal activity timestamps exercise stable ordering at page boundaries.
		if _, err := s.db.Exec("UPDATE sessions SET done_at = ?, created_at = ?, last_active_at = 100 WHERE id = ?", i, i, v.ID); err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	for offset := 0; offset < 6; offset += 2 {
		rows, err := s.ListSessions(SessionFilter{ProjectID: p.ID, OnlyDone: true, Limit: 2, Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	}
	if fmt.Sprint(ids) != "[6 5 4 3 2 1]" {
		t.Fatalf("pages: %v", ids)
	}
	rows, err := s.ListSessions(SessionFilter{ProjectID: p.ID, OnlyDone: true, Query: "task 2", Limit: 2})
	if err != nil || len(rows) != 1 || rows[0].ID != "2" {
		t.Fatalf("search: %v, %v", rows, err)
	}
	rows, err = s.ListSessions(SessionFilter{ProjectID: p.ID, Newest: true, Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].ID != "6" {
		t.Fatalf("newest: %v, %v", rows, err)
	}
	rows, err = s.ListSessions(SessionFilter{ProjectID: p.ID, ExcludeDone: true})
	if err != nil || len(rows) != 1 || rows[0].ID != "0" {
		t.Fatalf("open: %v, %v", rows, err)
	}
}

func BenchmarkArchivedSessions(b *testing.B) {
	s, err := Open(b.TempDir() + "/bench.db")
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	p, err := s.CreateProject("benchmark", b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		_, err = tx.Exec(`INSERT INTO sessions (id, project_id, title, provider, model, created_at, updated_at, last_active_at, done_at)
		 VALUES (?, ?, 'Archived task', 'fake', 'fake-quick', 1, 1, ?, 1)`, fmt.Sprint(i), p.ID, i)
		if err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	for _, limit := range []int{0, 26} {
		b.Run(fmt.Sprintf("limit=%d", limit), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := s.ListSessions(SessionFilter{ProjectID: p.ID, OnlyDone: true, Limit: limit}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
