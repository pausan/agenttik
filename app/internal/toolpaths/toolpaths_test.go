package toolpaths

import (
	"reflect"
	"testing"
)

func paths(root string, summaries ...string) []string {
	t := New(root)
	for _, s := range summaries {
		t.AddSummary(s)
	}
	return t.Paths()
}

func TestToolInputs(t *testing.T) {
	got := paths("/p",
		`Write {"file_path":"/p/tasks/a/build.py","content":"print(1)"}`,
		`Edit {"replace_all":false,"file_path":"/p/tasks/a/build.py","old_string":"x`,
		`view {"path":"games/g/config.json"}`,
		`NotebookEdit {"notebook_path":"n.ipynb"}`,
		`Read {"file_path":"/p/tasks/b/cut-of`,
	)
	want := []string{"/p/tasks/a/build.py", "/p/games/g/config.json", "/p/n.ipynb"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCommandsFollowCd(t *testing.T) {
	got := paths("/p",
		`Bash {"command":"cd /p/tasks/r && python3 build.py && rm preview.png && cat > README.md <<'EOF'\n# Report: `+"`x.html`"+` and more/words.txt\nEOF\nls -la out.csv","description":"Build"}`,
		`Bash {"command":"timeout 60 chromium --headless --screenshot=$PWD/shot.png \"file://$PWD/report.html\" 2>&1 | tail -2"}`,
	)
	want := []string{"/p/tasks/r", "/p/tasks/r/build.py", "/p/tasks/r/preview.png",
		"/p/tasks/r/README.md", "/p/tasks/r/out.csv", "/p/tasks/r/shot.png", "/p/tasks/r/report.html"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNewTurnResetsShell(t *testing.T) {
	tr := New("/p")
	tr.AddSummary(`Bash {"command":"cd sub"}`)
	tr.NewTurn()
	tr.AddSummary(`Bash {"command":"cat a.md"}`)
	if got := tr.Paths(); !reflect.DeepEqual(got, []string{"/p/sub", "/p/a.md"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCodexShellAndPatch(t *testing.T) {
	got := paths("/p",
		`shell /usr/bin/bash -lc 'python3 tasks/t/copy.py --cleanup'`,
		"apply_patch *** Begin Patch\n*** Update File: web/a.js\n@@\n-x\n+y\n*** Add File: /abs/b.md\n",
		`shell {"command":["bash","-lc","cd docs && cat c.md"],"workdir":"/w"}`,
	)
	want := []string{"/p/tasks/t/copy.py", "/p/web/a.js", "/abs/b.md", "/w/docs", "/w/docs/c.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIgnoresCodeAndNumbers(t *testing.T) {
	got := paths("/p",
		`Bash {"command":"python3 -c \"print(round(1*2.4))\" && node x.js --port 9531 && echo 2.4 s.v[li] https://a.com/b.html"}`,
	)
	if !reflect.DeepEqual(got, []string{"/p/x.js"}) {
		t.Fatalf("got %v", got)
	}
}

func TestRepeatedPathMovesToEnd(t *testing.T) {
	got := paths("/p", `Read {"file_path":"a.md"}`, `Read {"file_path":"b.md"}`, `Read {"file_path":"a.md"}`)
	if !reflect.DeepEqual(got, []string{"/p/b.md", "/p/a.md"}) {
		t.Fatalf("got %v", got)
	}
}
