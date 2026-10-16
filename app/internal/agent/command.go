package agent

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// CommandPrefix finds the part of a shell command an allow for the task can
// cover: the program, and its subcommand when the second word looks like one
// (`kubectl get`, `npm run`, `git status`, but `cat` for `cat notes.txt`).
// input is a tool approval's input JSON; only its "command" field is read.
//
// It is empty for anything that is not one plain command: chained or piped
// commands, redirections, substitutions, variable assignments in front, or
// quoting it cannot follow. Those are always asked, because a prefix says
// nothing about what the rest of the line does. A `bash -lc '...'` wrapper,
// which Codex may report, is looked through. See 082-tool-approvals.md.
func CommandPrefix(input string) string {
	words := commandWords(input)
	if len(words) == 0 || strings.Contains(words[0], "=") {
		return ""
	}
	if len(words) > 1 && subcommand.MatchString(words[1]) {
		return words[0] + " " + words[1]
	}
	return words[0]
}

// CommandHasPrefix reports whether the command in input is one plain command
// that starts with prefix's words.
func CommandHasPrefix(input, prefix string) bool {
	words, want := commandWords(input), strings.Fields(prefix)
	return len(want) > 0 && len(words) >= len(want) && slices.Equal(words[:len(want)], want)
}

var subcommand = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// commandWords splits the command in input into words, or returns nil when it
// is not one plain command.
func commandWords(input string) []string {
	var v struct {
		Command string `json:"command"`
	}
	if json.Unmarshal([]byte(input), &v) != nil {
		return nil
	}
	words := shellWords(v.Command)
	if len(words) == 3 && (words[1] == "-c" || words[1] == "-lc") {
		switch filepath.Base(words[0]) {
		case "bash", "sh", "zsh":
			words = shellWords(words[2])
		}
	}
	return words
}

// shellWords splits a command the way a shell would, for the simple cases
// only. Anything that would make the shell do more than run one program —
// operators, redirections, substitutions, globs' cousins like braces, an
// unclosed quote — returns nil.
func shellWords(command string) []string {
	var words []string
	var word strings.Builder
	inWord := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case c == '\'':
			end := strings.IndexByte(command[i+1:], '\'')
			if end < 0 {
				return nil
			}
			word.WriteString(command[i+1 : i+1+end])
			i += end + 1
			inWord = true
		case c == '"':
			end := strings.IndexByte(command[i+1:], '"')
			if end < 0 {
				return nil
			}
			quoted := command[i+1 : i+1+end]
			if strings.ContainsAny(quoted, "$`\\") {
				return nil
			}
			word.WriteString(quoted)
			i += end + 1
			inWord = true
		case strings.IndexByte(";&|<>`$(){}\\\n\r#", c) >= 0:
			return nil
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}
