package server

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
)

func TestTurnOutputs(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	base := time.UnixMilli(1_000_000_000_000)
	write := func(path string, at time.Duration) string {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, base.Add(at), base.Add(at)); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("tasks/r/build.py", 10*time.Second)
	write("tasks/r/report.html", 20*time.Second)
	write("tasks/r/shot.png", 30*time.Second)         // the agent's own check, never mentioned
	write("tasks/r/old.csv", -time.Hour)              // mentioned, but not written in any turn
	write("node_modules/x/chart.png", 20*time.Second) // dependencies are never listed
	tsv := write(filepath.Join(outside, "slow.tsv"), 40*time.Second)
	write("tasks/r/table.csv", 140*time.Second)

	ms := func(d time.Duration) int64 { return base.Add(d).UnixMilli() }
	turns := []store.Turn{
		{ID: 1, StartedAt: ms(0), EndedAt: ms(60 * time.Second)},
		{ID: 2, StartedAt: ms(100 * time.Second), EndedAt: ms(150 * time.Second)},
		{ID: 3, StartedAt: ms(200 * time.Second), EndedAt: ms(210 * time.Second)},
	}
	messages := []store.Message{
		{TurnID: 1, Role: store.RoleTool, Content: `Bash {"command":"cd tasks/r && python3 build.py && ls ../../node_modules/x"}`},
		{TurnID: 1, Role: store.RoleTool, Content: `Read {"file_path":"` + tsv + `"}`},
		{TurnID: 1, Role: store.RoleAssistant, Content: "Report: `report.html`, rows in " + tsv + ", old `tasks/r/old.csv`, chart.png"},
		{TurnID: 2, Role: store.RoleTool, Content: `Bash {"command":"python3 tasks/r/build.py"}`},
		{TurnID: 2, Role: store.RoleAssistant, Content: "Done, see [the table](./tasks/r/table.csv)."},
		{TurnID: 3, Role: store.RoleAssistant, Content: "Nothing written: `tasks/r/report.html`."},
	}
	got := turnOutputs(root, turns, messages)
	type row struct {
		turn       int64
		path, kind string
	}
	rows := []row{}
	for _, o := range got {
		rows = append(rows, row{o.TurnID, o.Path, o.Kind})
	}
	want := []row{{1, "tasks/r/report.html", "html"}, {1, filepath.ToSlash(tsv), "table"}, {2, "tasks/r/table.csv", "table"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %v, want %v", rows, want)
	}

	// Rewritten in turn 2, the report moves there although that reply only
	// mentioned the table: it was presented in turn 1.
	write("tasks/r/report.html", 120*time.Second)
	got = turnOutputs(root, turns, messages)
	if len(got) != 3 || got[1].TurnID != 2 || got[1].Path != "tasks/r/report.html" || got[1].Modified != ms(120*time.Second) {
		t.Fatalf("rewritten report: %+v", got)
	}
}
