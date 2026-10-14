// Package toolpaths reads which files and folders an agent's tool calls name.
// A reply often says only `report.md` after the agent has worked in a
// subfolder; its own calls say where. See specs/022-transcript-file-links.md.
package toolpaths

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Tracker follows one turn's calls in order. A `cd` in one shell call moves
// where the next call's relative paths resolve, as the agent's shell does.
type Tracker struct {
	root  string
	cwd   string
	order []string
	at    map[string]int
}

// New starts at the agent's working folder.
func New(root string) *Tracker {
	root = filepath.Clean(root)
	return &Tracker{root: root, cwd: root, at: map[string]int{}}
}

// NewTurn returns the shell to the working folder: each turn is a new process.
func (t *Tracker) NewTurn() { t.cwd = t.root }

// Paths are absolute and cleaned, oldest mention first; a path named again
// moves to the end.
func (t *Tracker) Paths() []string {
	out := make([]string, 0, len(t.at))
	for i, p := range t.order {
		if t.at[p] == i {
			out = append(out, p)
		}
	}
	return out
}

// AddSummary reads a stored transcript row: the tool name, a space, and its
// input, cut at a few hundred bytes.
func (t *Tracker) AddSummary(summary string) {
	name, input, _ := strings.Cut(summary, " ")
	t.Add(name, input)
}

// Add reads one call. Inputs are JSON objects for most tools, a raw patch
// for apply_patch and a raw command line for Codex's shell.
func (t *Tracker) Add(name, input string) {
	input = strings.TrimSpace(input)
	switch {
	case strings.HasPrefix(input, "{"):
		t.addObject(input)
	case strings.HasPrefix(input, "*** Begin Patch"):
		for _, m := range patchFile.FindAllStringSubmatch(input, -1) {
			t.add(t.cwd, strings.TrimSpace(m[1]))
		}
	case name == "shell" || name == "bash" || name == "Bash":
		t.command(input)
	}
}

var (
	pathKeys = map[string]bool{"path": true, "file_path": true, "filePath": true,
		"notebook_path": true, "filename": true, "file": true, "dir": true, "directory": true}
	cwdKeys   = map[string]bool{"cwd": true, "workdir": true}
	patchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add File|Update File|Move to): (.+)$`)
	// A summary is cut at a fixed length, so its JSON may not parse. Keys
	// come first, which is where tools put paths.
	jsonString = regexp.MustCompile(`"(\w+)"\s*:\s*"((?:[^"\\]|\\.)*)("?)`)
	extension  = regexp.MustCompile(`\.[0-9A-Za-z]*[A-Za-z][0-9A-Za-z]*$`)
)

func (t *Tracker) addObject(input string) {
	var fields map[string]any
	if json.Unmarshal([]byte(input), &fields) != nil {
		fields = map[string]any{}
		for _, m := range jsonString.FindAllStringSubmatch(input, -1) {
			var value string
			if json.Unmarshal([]byte(`"`+m[2]+`"`), &value) != nil {
				continue
			}
			// A cut path names a file that does not exist; a cut command
			// still says what came before the cut.
			if m[3] == "" && !(m[1] == "command" || m[1] == "cmd") {
				continue
			}
			fields[m[1]] = value
		}
	}
	base := t.cwd
	for key, value := range fields {
		if s, ok := value.(string); ok && cwdKeys[key] {
			base = t.resolve(t.cwd, s)
		}
	}
	for key, value := range fields {
		switch v := value.(type) {
		case string:
			if pathKeys[key] {
				t.add(base, v)
			} else if key == "command" || key == "cmd" {
				next := t.commandIn(base, v)
				if base == t.cwd {
					t.cwd = next
				}
			}
		case []any:
			if key == "command" || key == "cmd" {
				words := []string{}
				for _, w := range v {
					if s, ok := w.(string); ok {
						words = append(words, s)
					}
				}
				t.words(base, words)
			}
		}
	}
}

func (t *Tracker) command(line string) { t.cwd = t.commandIn(t.cwd, line) }

// commandIn walks a shell line segment by segment and returns where it left
// the shell. Heredoc bodies are data, not paths, and are skipped.
func (t *Tracker) commandIn(cwd, line string) string {
	var segment []string
	flush := func() {
		cwd = t.words(cwd, segment)
		segment = segment[:0]
	}
	heredoc := ""
	for _, raw := range strings.Split(line, "\n") {
		if heredoc != "" {
			if strings.TrimSpace(raw) == heredoc {
				heredoc = ""
			}
			continue
		}
		words, end := shellWords(raw)
		for _, w := range words {
			switch w {
			case "&&", "||", ";", "|", "&":
				flush()
			default:
				segment = append(segment, w)
			}
		}
		heredoc = end
		flush()
	}
	return cwd
}

// words reads one simple command. A quoted argument with spaces is usually a
// nested command (`bash -lc '…'`), so it is read as one in the same folder.
func (t *Tracker) words(cwd string, words []string) string {
	if len(words) == 0 {
		return cwd
	}
	if words[0] == "cd" {
		if len(words) > 1 && words[1] != "-" {
			cwd = t.resolve(cwd, words[1])
			t.add(cwd, ".")
		} else if len(words) == 1 {
			cwd = t.root
		}
		return cwd
	}
	for _, w := range words[1:] {
		if strings.ContainsAny(w, " \t") {
			t.commandIn(cwd, w)
			continue
		}
		if strings.HasPrefix(w, "-") {
			// --screenshot=out/a.png names a file; -n does not.
			_, after, ok := strings.Cut(w, "=")
			if !ok {
				continue
			}
			w = after
		}
		w = strings.ReplaceAll(strings.ReplaceAll(w, "${PWD}", cwd), "$PWD", cwd)
		w = strings.TrimPrefix(w, "file://")
		if pathLike(w) {
			t.add(cwd, w)
		}
	}
	return cwd
}

// pathLike keeps words that name a file or folder: ones with a slash or a
// file extension, made only of characters paths are written with.
func pathLike(w string) bool {
	if w == "" || w == "." || w == ".." || strings.Contains(w, "://") {
		return false
	}
	for _, r := range w {
		if strings.ContainsRune("$`\"'(){}[]*?<>|&;!\\,=", r) {
			return false
		}
	}
	return strings.Contains(w, "/") || extension.MatchString(w)
}

func (t *Tracker) resolve(cwd, path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = home + path[1:]
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

func (t *Tracker) add(cwd, path string) {
	if path = strings.TrimSpace(path); path == "" {
		return
	}
	p := t.resolve(cwd, path)
	t.at[p] = len(t.order)
	t.order = append(t.order, p)
}

// shellWords splits one line the way a shell would, enough to find paths:
// quotes group, operators stand alone, a comment ends the line. It also
// reports a heredoc delimiter, whose body starts on the next line.
func shellWords(line string) (words []string, heredoc string) {
	var cur strings.Builder
	in := false
	quote := rune(0)
	push := func() {
		if in {
			words = append(words, cur.String())
		}
		cur.Reset()
		in = false
	}
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' && i+1 < len(rs) {
				i++
				cur.WriteRune(rs[i])
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, in = r, true
		case r == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			in = true
		case r == ' ' || r == '\t':
			push()
		case r == '#' && !in:
			push()
			return words, heredoc
		case r == '<' && i+1 < len(rs) && rs[i+1] == '<':
			push()
			i += 2
			if i < len(rs) && rs[i] == '-' {
				i++
			}
			for i < len(rs) && rs[i] == ' ' {
				i++
			}
			start := i
			for i < len(rs) && !strings.ContainsRune(" \t;|&<>", rs[i]) {
				i++
			}
			heredoc = strings.Trim(string(rs[start:i]), `'"`)
			i--
		case strings.ContainsRune(";|&<>", r):
			push()
			op := string(r)
			if (r == '&' || r == '|') && i+1 < len(rs) && rs[i+1] == r {
				op += string(r)
				i++
			}
			if r != '<' && r != '>' {
				words = append(words, op)
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	push()
	return words, heredoc
}
