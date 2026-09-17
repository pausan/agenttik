package terminals

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The shell is the user's own, and a test cannot know what that prints. Every
// test here runs /bin/sh instead, by naming it in the environment the manager
// reads, so what a prompt looks like never enters into it.
func useSh(t *testing.T) {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")
}

func open(t *testing.T, m *Manager, dir string) Info {
	t.Helper()
	info, err := m.Open(1, dir, 80, 24)
	if err != nil {
		t.Skipf("no pseudo-terminal available here: %v", err)
	}
	t.Cleanup(func() { m.Close(info.ID) })
	return info
}

// await reads frames until want shows up in the output, or gives up. A shell
// writes its own prompt and echoes what is typed, so the answer arrives in
// pieces and after some of them.
func await(t *testing.T, frames <-chan Frame, want string) string {
	t.Helper()
	var seen strings.Builder
	deadline := time.After(10 * time.Second)
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("stream ended before %q; saw %q", want, seen.String())
			}
			if frame.Data != "" {
				b, err := base64.StdEncoding.DecodeString(frame.Data)
				if err != nil {
					t.Fatalf("frame is not base64: %v", err)
				}
				seen.Write(b)
			}
			if strings.Contains(seen.String(), want) {
				return seen.String()
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q; saw %q", want, seen.String())
		}
	}
}

// The point of the whole thing: what is typed runs in the project's folder and
// what it prints comes back.
func TestTerminalRunsCommandsInTheProjectFolder(t *testing.T) {
	useSh(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	info := open(t, m, dir)
	frames, stop := m.SubscribeMany([]string{info.ID})
	defer stop()

	if err := m.Write(info.ID, []byte("ls\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	await(t, frames, "marker.txt")
}

// A watcher arriving after the output was written is handed the screen rather
// than a blank one. This is what switching back to a project does.
func TestLateWatcherIsHandedTheScrollback(t *testing.T) {
	useSh(t)
	m := NewManager()
	info := open(t, m, t.TempDir())

	first, stop := m.SubscribeMany([]string{info.ID})
	if err := m.Write(info.ID, []byte("echo written-before-watching\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	await(t, first, "written-before-watching")
	stop()

	later, stopLater := m.SubscribeMany([]string{info.ID})
	defer stopLater()
	await(t, later, "written-before-watching")
}

// One connection carries every terminal, each frame naming its own.
func TestOneStreamCarriesSeveralTerminals(t *testing.T) {
	useSh(t)
	m := NewManager()
	one := open(t, m, t.TempDir())
	two := open(t, m, t.TempDir())

	frames, stop := m.SubscribeMany([]string{one.ID, two.ID})
	defer stop()

	if err := m.Write(two.ID, []byte("echo from-the-second\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	for {
		select {
		case frame := <-frames:
			if frame.ID != two.ID {
				continue
			}
			b, _ := base64.StdEncoding.DecodeString(frame.Data)
			if strings.Contains(string(b), "from-the-second") {
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the second terminal's output never arrived on the shared stream")
		}
	}
}

// Resizing tells the shell, which is how a full-screen program redraws to fit.
func TestResizeReachesTheShell(t *testing.T) {
	useSh(t)
	m := NewManager()
	info := open(t, m, t.TempDir())
	frames, stop := m.SubscribeMany([]string{info.ID})
	defer stop()

	if err := m.Resize(info.ID, 132, 43); err != nil {
		t.Fatalf("resize: %v", err)
	}
	// The shell reads its size when it is asked, so the question is put after
	// the resize rather than relying on a signal it may not have handled yet.
	if err := m.Write(info.ID, []byte("stty size\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	await(t, frames, "43 132")
}

// Closing ends the shell, and the stream says so rather than going quiet.
func TestCloseEndsTheShellAndSaysSo(t *testing.T) {
	useSh(t)
	m := NewManager()
	info, err := m.Open(1, t.TempDir(), 80, 24)
	if err != nil {
		t.Skipf("no pseudo-terminal available here: %v", err)
	}
	frames, stop := m.SubscribeMany([]string{info.ID})
	defer stop()

	if err := m.Close(info.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	if m.List(1) != nil && len(m.List(1)) != 0 {
		t.Error("a closed terminal is still listed")
	}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case frame, ok := <-frames:
			// A closed pseudo-terminal ends the reader, which is either an
			// exit frame or the stream finishing. Both say the same thing.
			if !ok || frame.Exit {
				return
			}
		case <-deadline:
			t.Fatal("closing never reached the watcher")
		}
	}
}

// Writing to a terminal that is gone is a 404, not a panic.
func TestUnknownTerminal(t *testing.T) {
	m := NewManager()
	if err := m.Write("nope", []byte("x")); err != ErrNotFound {
		t.Errorf("write to a missing terminal: got %v, want %v", err, ErrNotFound)
	}
	if err := m.Resize("nope", 80, 24); err != ErrNotFound {
		t.Errorf("resize of a missing terminal: got %v, want %v", err, ErrNotFound)
	}
	if err := m.Close("nope"); err != ErrNotFound {
		t.Errorf("close of a missing terminal: got %v, want %v", err, ErrNotFound)
	}
}

// A project's terminals are listed oldest first, which is the order the UI
// draws their tabs in, and one project never sees another's.
func TestListIsPerProjectAndInOpeningOrder(t *testing.T) {
	useSh(t)
	m := NewManager()
	defer m.Shutdown()

	dir := t.TempDir()
	first, err := m.Open(7, dir, 80, 24)
	if err != nil {
		t.Skipf("no pseudo-terminal available here: %v", err)
	}
	second, err := m.Open(7, dir, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.Open(8, dir, 80, 24)
	if err != nil {
		t.Fatal(err)
	}

	got := m.List(7)
	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("project 7 should list its two terminals in order, got %+v", got)
	}
	if got[0].Title == got[1].Title {
		t.Errorf("two terminals in one project share the name %q", got[0].Title)
	}
	if list := m.List(8); len(list) != 1 || list[0].ID != other.ID {
		t.Errorf("project 8 should list only its own terminal, got %+v", list)
	}
}

// Deleting a project takes its terminals with it and leaves everyone else's.
func TestCloseProject(t *testing.T) {
	useSh(t)
	m := NewManager()
	defer m.Shutdown()

	dir := t.TempDir()
	if _, err := m.Open(1, dir, 80, 24); err != nil {
		t.Skipf("no pseudo-terminal available here: %v", err)
	}
	kept, err := m.Open(2, dir, 80, 24)
	if err != nil {
		t.Fatal(err)
	}

	m.CloseProject(1)
	if list := m.List(1); len(list) != 0 {
		t.Errorf("the deleted project still has terminals: %+v", list)
	}
	if list := m.List(2); len(list) != 1 || list[0].ID != kept.ID {
		t.Errorf("another project's terminal was closed with it: %+v", list)
	}
}

// A folder that is not there cannot hold a shell, and saying so is better than
// starting one somewhere else.
func TestOpenNeedsAFolder(t *testing.T) {
	m := NewManager()
	if _, err := m.Open(1, filepath.Join(t.TempDir(), "gone"), 80, 24); err == nil {
		t.Error("opening a terminal in a missing folder should fail")
	}
	if _, err := m.Open(1, "", 80, 24); err == nil {
		t.Error("opening a terminal with no folder should fail")
	}
}

// The scrollback is capped, so a terminal that has printed a gigabyte is still
// a few hundred kilobytes of memory.
func TestScrollbackIsCapped(t *testing.T) {
	term := &Terminal{streams: make(map[*stream]struct{})}
	chunk := make([]byte, readChunk)
	for i := 0; i < (4*scrollback)/readChunk; i++ {
		term.remember(chunk)
	}
	if len(term.back) > 2*scrollback {
		t.Errorf("scrollback grew to %d bytes, past the %d ceiling", len(term.back), 2*scrollback)
	}
	if len(term.back) < scrollback {
		t.Errorf("scrollback kept only %d bytes, less than the %d it should", len(term.back), scrollback)
	}
}

// A terminal is named after the shell running in it. The separator is this
// platform's, since the path only ever comes from this platform's PATH; the
// extension is Windows' and is dropped wherever it turns up.
func TestLabel(t *testing.T) {
	for name, want := range map[string]string{
		filepath.Join("bin", "bash"):                 "bash",
		filepath.Join("usr", "local", "bin", "fish"): "fish",
		"cmd.exe":        "cmd",
		"powershell.exe": "powershell",
		"":               "Terminal",
	} {
		if got := label(name); got != want {
			t.Errorf("label(%q) = %q, want %q", name, got, want)
		}
	}
}

// Anything using curses refuses to start without TERM, so the shell is always
// given one even when this process has none.
func TestEnvironNamesTheTerminalType(t *testing.T) {
	t.Setenv("TERM", "dumb")
	var term, color int
	for _, v := range environ() {
		if v == "TERM=xterm-256color" {
			term++
		}
		if strings.HasPrefix(v, "COLORTERM=") {
			color++
		}
		if v == "TERM=dumb" {
			t.Error("the shell inherited this process's TERM")
		}
	}
	if term != 1 {
		t.Errorf("TERM should be set exactly once, got %d", term)
	}
	if color != 1 {
		t.Errorf("COLORTERM should be set exactly once, got %d", color)
	}
}

// A size nobody measured still has to be one a shell can draw in.
func TestSizeDefaults(t *testing.T) {
	if cols, rows := size(0, 0); cols != 80 || rows != 24 {
		t.Errorf("unmeasured size = %dx%d, want 80x24", cols, rows)
	}
	if cols, rows := size(-5, 100000); cols != 80 || rows != 1000 {
		t.Errorf("nonsense size = %dx%d, want 80x1000", cols, rows)
	}
}
