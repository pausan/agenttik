// Package terminals runs interactive shells on pseudo-terminals, one per
// terminal tab in the UI, and fans their output out to whoever is watching.
//
// This is the opposite side of server/terminal.go: that one hands a command to
// the desktop's own terminal application, which is right for a CLI login flow
// nobody watches through the UI. This one is the terminal, inside the window,
// in the project's folder.
//
// A terminal belongs to a project and to this process. Nothing about it
// reaches SQLite: a shell is a running thing, and a row saying one used to
// exist would only ever describe a process that had already gone. Quitting the
// app therefore ends every terminal, which is what closing a terminal window
// does anywhere else.
package terminals

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	xpty "github.com/aymanbagabas/go-pty"
	"github.com/google/uuid"

	"github.com/pausan/agenttik/app/internal/process"
)

// scrollback is how much of a terminal's output is kept so that a watcher
// arriving late — a project switched back to, a reloaded window — can be
// handed the screen as it stands rather than a blank one. 256KB is several
// screens of a build log and costs nothing next to the shell itself.
//
// The replay is a byte slice cut at an arbitrary point, so a very old escape
// sequence can be halved by the cut. A terminal emulator discards a partial
// sequence, which is why the cut is allowed to be this careless.
const scrollback = 256 << 10

// readChunk bounds one read off the pty. Output arrives as fast as the shell
// writes it, so this is a ceiling on a frame rather than a target.
const readChunk = 32 << 10

// streamBuffer is how many frames a watcher may fall behind before its stream
// is closed. Closing it is not a loss: the browser reconnects and is handed
// the scrollback, so the screen comes back rather than being missed.
const streamBuffer = 512

// exitGrace is how long a closing shell is given to take the hangup before it
// is killed outright.
const exitGrace = 2 * time.Second

// ErrNotFound is returned for a terminal that has been closed or never was.
var ErrNotFound = errors.New("terminal not found")

// Frame is one piece of news about one terminal: output, or the shell ending.
// Data is base64 because a terminal's output is bytes, not text, and this
// travels as JSON over SSE.
//
// Reset says the frame is a whole screen rather than the next piece of one,
// so the watcher clears what it has before writing it. See SubscribeMany.
//
// At is the shell's byte count once this frame has been applied, and is what
// the watcher sends back as Watch.From. It is carried rather than counted by
// the watcher because a reset frame is the tail of a longer history: the bytes
// in it say how much was sent, not how much has happened.
type Frame struct {
	ID    string `json:"id"`
	Data  string `json:"data,omitempty"`
	At    int64  `json:"at"`
	Exit  bool   `json:"exit,omitempty"`
	Reset bool   `json:"reset,omitempty"`
}

// Watch is one terminal a stream wants, and how much of its output the watcher
// already has. From counts bytes since the shell started, which is what each
// Frame.Data advances; a watcher with nothing sends zero.
type Watch struct {
	ID   string
	From int64
}

// Info is a terminal as the UI lists it.
type Info struct {
	ID        string `json:"id"`
	ProjectID int64  `json:"project_id"`
	Title     string `json:"title"`
	Dir       string `json:"dir"`
	CreatedAt int64  `json:"created_at"`
	Exited    bool   `json:"exited"`
}

// Manager owns every open terminal, in the order they were opened. That order
// is what the UI rebuilds its tabs from, so it is kept here rather than left
// to a map's iteration.
type Manager struct {
	mu   sync.Mutex
	open []*Terminal
	// counters is the per-project number a terminal is named by, so two
	// shells in one project are told apart by something stable.
	counters map[int64]int
}

func NewManager() *Manager {
	return &Manager{counters: make(map[int64]int)}
}

// Terminal is one shell on one pseudo-terminal.
type Terminal struct {
	id        string
	projectID int64
	title     string
	dir       string
	created   time.Time

	pty xpty.Pty
	cmd *xpty.Cmd

	mu   sync.Mutex
	back []byte
	// written is every byte the shell has ever produced, of which back holds
	// the tail. The two together let a watcher say where it got to and be
	// given only what it missed.
	written int64
	streams map[*stream]struct{}
	exited  bool
	closed  bool
}

func (t *Terminal) info() Info {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Info{
		ID:        t.id,
		ProjectID: t.projectID,
		Title:     t.title,
		Dir:       t.dir,
		CreatedAt: t.created.UnixMilli(),
		Exited:    t.exited,
	}
}

// Open starts a shell in dir and begins pumping its output. cols and rows are
// the size of the terminal the UI has room for; a client that has not measured
// itself yet may send zero and resize once it has.
func (m *Manager) Open(projectID int64, dir string, cols, rows int) (Info, error) {
	if dir == "" {
		return Info{}, errors.New("no folder to open a terminal in")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return Info{}, fmt.Errorf("cannot open a terminal in %s", dir)
	}
	cols, rows = size(cols, rows)

	p, err := xpty.New()
	if err != nil {
		return Info{}, fmt.Errorf("no pseudo-terminal available: %w", err)
	}
	// Sized before the shell starts, so the prompt it draws first is already
	// laid out for the window rather than for an 80x24 default it would have
	// to be told about afterwards.
	if err := p.Resize(cols, rows); err != nil {
		p.Close()
		return Info{}, err
	}

	name, args := shell()
	cmd := p.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = environ()
	if err := cmd.Start(); err != nil {
		p.Close()
		return Info{}, fmt.Errorf("cannot start %s: %w", name, err)
	}
	// Start has handed the slave end to the child, and this process holding a
	// copy of it open is what would keep the master readable forever: a shell
	// that exited on its own would never reach the end of its output, so the
	// tab would sit there on a dead screen waiting for more. Windows has no
	// second end to let go of — its output pipe closes with the process.
	if unix, ok := p.(xpty.UnixPty); ok {
		unix.Slave().Close()
	}

	m.mu.Lock()
	m.counters[projectID]++
	n := m.counters[projectID]
	m.mu.Unlock()

	t := &Terminal{
		id:        uuid.NewString(),
		projectID: projectID,
		title:     fmt.Sprintf("%s %d", label(name), n),
		dir:       dir,
		created:   time.Now(),
		pty:       p,
		cmd:       cmd,
		streams:   make(map[*stream]struct{}),
	}

	m.mu.Lock()
	m.open = append(m.open, t)
	m.mu.Unlock()

	go t.pump()
	return t.info(), nil
}

// size clamps a requested terminal size to something a shell can draw in. A
// client that has not measured itself sends zero, and every terminal that ever
// had no better answer has used 80x24.
func size(cols, rows int) (int, int) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	// A pseudo-terminal's dimensions are 16-bit, and a pane cannot be this
	// big anyway; the clamp is against a nonsense request, not a real one.
	if cols > 1000 {
		cols = 1000
	}
	if rows > 1000 {
		rows = 1000
	}
	return cols, rows
}

// pump reads the shell's output until it ends, keeping the scrollback and
// handing each read to every watcher.
func (t *Terminal) pump() {
	buf := make([]byte, readChunk)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			t.publish(buf[:n])
		}
		if err != nil {
			break
		}
	}
	// Reap the shell so a finished one does not sit as a zombie for the life
	// of the app. Its status is not reported: a terminal shows its own exit.
	if t.cmd != nil {
		go t.cmd.Wait()
	}
	t.finish()
}

// publish appends to the scrollback and sends one frame to each watcher.
func (t *Terminal) publish(b []byte) {
	data := base64.StdEncoding.EncodeToString(b)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.remember(b)
	frame := Frame{ID: t.id, Data: data, At: t.written}
	for s := range t.streams {
		if !s.send(frame) {
			delete(t.streams, s)
		}
	}
}

// remember keeps the tail of the output. The slice is allowed to grow to twice
// the limit before it is cut back to it, so the copy that cuts it happens once
// per scrollback's worth of output rather than once per read.
func (t *Terminal) remember(b []byte) {
	t.written += int64(len(b))
	t.back = append(t.back, b...)
	if len(t.back) <= 2*scrollback {
		return
	}
	t.back = append(t.back[:0], t.back[len(t.back)-scrollback:]...)
}

// finish marks the shell gone and tells every watcher once.
func (t *Terminal) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.exited = true
	for s := range t.streams {
		s.send(Frame{ID: t.id, At: t.written, Exit: true})
		delete(t.streams, s)
	}
}

// Write sends keystrokes to a terminal.
func (m *Manager) Write(id string, b []byte) error {
	t := m.find(id)
	if t == nil {
		return ErrNotFound
	}
	if len(b) == 0 {
		return nil
	}
	_, err := t.pty.Write(b)
	return err
}

// Resize tells the shell its window changed, which is how a full-screen
// program redraws itself at the new size.
func (m *Manager) Resize(id string, cols, rows int) error {
	t := m.find(id)
	if t == nil {
		return ErrNotFound
	}
	cols, rows = size(cols, rows)
	return t.pty.Resize(cols, rows)
}

// List is one project's terminals, oldest first, which is the order their tabs
// are drawn in.
func (m *Manager) List(projectID int64) []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.open))
	for _, t := range m.open {
		if t.projectID == projectID {
			out = append(out, t.info())
		}
	}
	return out
}

// Close ends one terminal.
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	var found *Terminal
	for i, t := range m.open {
		if t.id == id {
			found = t
			m.open = append(m.open[:i], m.open[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	if found == nil {
		return ErrNotFound
	}
	found.stop()
	return nil
}

// CloseProject ends every terminal of one project, which is what deleting the
// project does to them.
func (m *Manager) CloseProject(projectID int64) {
	m.mu.Lock()
	var going []*Terminal
	kept := m.open[:0]
	for _, t := range m.open {
		if t.projectID == projectID {
			going = append(going, t)
			continue
		}
		kept = append(kept, t)
	}
	m.open = kept
	delete(m.counters, projectID)
	m.mu.Unlock()
	for _, t := range going {
		t.stop()
	}
}

// Shutdown ends every terminal. The app is quitting, and a shell outliving the
// window it was typed into would hold the project folder open with nothing
// left to show it.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	going := m.open
	m.open = nil
	m.mu.Unlock()
	for _, t := range going {
		t.stop()
	}
}

func (m *Manager) find(id string) *Terminal {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.open {
		if t.id == id {
			return t
		}
	}
	return nil
}

// stop closes the pseudo-terminal, which hangs the shell up, and kills
// whatever is still there shortly after. Closing the master end alone is
// enough for a shell sitting at its prompt; a foreground program ignoring the
// hangup is what the kill is for. Either way the process group goes, so a
// build running in the terminal ends with it rather than carrying on
// unattached.
func (t *Terminal) stop() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.mu.Unlock()

	t.pty.Close()
	p := t.cmd.Process
	if p == nil {
		return
	}
	process.Terminate(p)
	go func() {
		time.Sleep(exitGrace)
		process.Kill(p)
	}()
}

// SubscribeMany registers one channel for several terminals at once, the way
// the event hub does and for the same reason: a window with three terminals
// open watches them over one connection, because a browser will not hold a
// connection per tab.
//
// Each watcher says how much of each terminal it already has, and is sent only
// what it missed. That matters because the stream is reopened whenever the set
// of terminals on it changes: opening a second terminal must not repeat the
// first one's output into a view already showing it. A watcher with nothing,
// or one so far behind that what it missed has been trimmed away, is handed
// the scrollback as a fresh screen instead.
func (m *Manager) SubscribeMany(watches []Watch) (<-chan Frame, func()) {
	s := &stream{ch: make(chan Frame, streamBuffer)}

	var watched []*Terminal
	seen := make(map[string]bool, len(watches))
	for _, w := range watches {
		if seen[w.ID] {
			continue
		}
		seen[w.ID] = true
		if t := m.find(w.ID); t != nil {
			watched = append(watched, t)
			t.attach(s, w.From)
		}
	}

	return s.ch, func() {
		for _, t := range watched {
			t.detach(s)
		}
		s.close()
	}
}

// attach hands over what the watcher missed and then registers it for what
// comes next, both under the lock publish takes, so nothing arrives between
// the two and nothing arrives out of order.
func (t *Terminal) attach(s *stream, from int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	missed, reset := t.since(from)
	if len(missed) > 0 {
		s.send(Frame{ID: t.id, Data: base64.StdEncoding.EncodeToString(missed), At: t.written, Reset: reset})
	}
	if t.exited {
		s.send(Frame{ID: t.id, At: t.written, Exit: true})
		return
	}
	t.streams[s] = struct{}{}
}

// since is the output produced after the watcher's byte count, and whether
// that is a whole screen rather than a continuation of one. The caller holds
// the lock.
func (t *Terminal) since(from int64) (missed []byte, reset bool) {
	// A count level with everything written has missed nothing. One past the
	// end is a client holding a number from some other terminal's life, and
	// starting it over is the honest answer to that.
	if from == t.written {
		return nil, false
	}
	oldest := t.written - int64(len(t.back))
	if from >= oldest && from < t.written {
		return t.back[from-oldest:], false
	}
	return t.back, true
}

func (t *Terminal) detach(s *stream) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.streams, s)
}

// stream is one watcher's channel. Several terminals' readers publish into it
// at once, so the flag that retires it and the close that ends it are both
// under its own lock.
type stream struct {
	mu   sync.Mutex
	ch   chan Frame
	dead bool
}

func (s *stream) send(f Frame) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return false
	}
	select {
	case s.ch <- f:
		return true
	default:
		// Too far behind to catch up. Ending the stream is how the browser
		// learns to reconnect and be given the screen again.
		s.dead = true
		close(s.ch)
		return false
	}
}

func (s *stream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return
	}
	s.dead = true
	close(s.ch)
}

// shell is the program a new terminal runs. It is the user's own shell
// wherever the system names one, because a terminal that is not the shell you
// configured is a terminal with the wrong aliases, prompt and history.
//
// No login flag: started on a tty with no command to run, every common shell
// is already interactive and reads its interactive startup file. Asking for a
// login shell as well would re-read the profile on every tab.
func shell() (string, []string) {
	if runtime.GOOS == "windows" {
		for _, candidate := range []string{"pwsh.exe", "powershell.exe", "cmd.exe"} {
			if path, err := exec.LookPath(candidate); err == nil {
				return path, nil
			}
		}
		return "cmd.exe", nil
	}
	if sh := strings.TrimSpace(os.Getenv("SHELL")); sh != "" {
		return sh, nil
	}
	for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "/bin/sh", nil
}

// label names a terminal after the shell in it, so a tab says what it is.
func label(name string) string {
	base := filepath.Base(name)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "Terminal"
	}
	return base
}

// environ is the shell's environment: this process's, plus the two variables
// that tell a program what it is drawing on. Without TERM, anything using
// curses refuses to start; without COLORTERM, the ones that check it fall back
// to 256 colours.
func environ() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+2)
	for _, v := range env {
		if strings.HasPrefix(v, "TERM=") || strings.HasPrefix(v, "COLORTERM=") {
			continue
		}
		out = append(out, v)
	}
	return append(out, "TERM=xterm-256color", "COLORTERM=truecolor")
}
