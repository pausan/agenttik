package agent

import (
	"encoding/json"
	"testing"
)

func commandInput(command string) string {
	b, _ := json.Marshal(map[string]string{"command": command, "description": "d"})
	return string(b)
}

func TestCommandPrefix(t *testing.T) {
	for command, want := range map[string]string{
		"kubectl get pods -n kube-system": "kubectl get",
		"npm run build":                   "npm run",
		"git status":                      "git status",
		"ls":                              "ls",
		"ls -la":                          "ls",
		"cat notes.txt":                   "cat",
		"cat README":                      "cat",
		"/usr/bin/kubectl get pods":       "/usr/bin/kubectl get",
		`grep -r 'a b' .`:                 "grep",
		`bash -lc 'kubectl get pods'`:     "kubectl get",
		`/bin/zsh -c "git log"`:           "git log",
		// Not one plain command: always asked.
		"npm run build && ls dist":   "",
		"ls | wc -l":                 "",
		"kubectl get pods; rm -rf /": "",
		"echo hi > out.txt":          "",
		"echo $(whoami)":             "",
		"echo `whoami`":              "",
		`echo "$HOME"`:               "",
		"FOO=1 kubectl get pods":     "",
		"echo 'unclosed":             "",
		"ls\nrm -rf /":               "",
		"":                           "",
	} {
		if got := CommandPrefix(commandInput(command)); got != want {
			t.Errorf("%q: prefix %q, want %q", command, got, want)
		}
	}
	if got := CommandPrefix(`{"file_path":"/x"}`); got != "" {
		t.Errorf("no command: prefix %q", got)
	}
}

func TestCommandHasPrefix(t *testing.T) {
	for command, want := range map[string]bool{
		"kubectl get pods":           true,
		"kubectl get deployments -A": true,
		"kubectl get":                true,
		"kubectl delete pod x":       false,
		"kubectl getter":             false,
		"kubectl get pods && rm x":   false,
		`sh -c 'kubectl get nodes'`:  true,
	} {
		if got := CommandHasPrefix(commandInput(command), "kubectl get"); got != want {
			t.Errorf("%q: %v, want %v", command, got, want)
		}
	}
	if CommandHasPrefix(commandInput("ls"), "") {
		t.Error("an empty prefix matched")
	}
}
