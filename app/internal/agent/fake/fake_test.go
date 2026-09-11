package fake

import "testing"

func TestCutDirective(t *testing.T) {
	cases := []struct {
		line string
		name string
		rest string
		ok   bool
	}{
		{"@wait 300", "wait", "300", true},
		{"  @tool Bash echo hi", "tool", "Bash echo hi", true},
		{"@error rate limit reached", "error", "rate limit reached", true},
		{"@bogus x", "", "", false}, // not a known directive: kept as text
		{"plain reply line", "", "", false},
		{"@", "", "", false},
	}
	for _, c := range cases {
		name, rest, ok := cutDirective(c.line)
		if name != c.name || rest != c.rest || ok != c.ok {
			t.Errorf("cutDirective(%q) = %q, %q, %v; want %q, %q, %v",
				c.line, name, rest, ok, c.name, c.rest, c.ok)
		}
	}
}

func TestTitleSubject(t *testing.T) {
	cases := []struct {
		name    string
		wrapped string
		want    string
	}{
		{"wrapped prompt", "Name the task.\n\n<user-request>\nfix the flaky test\n</user-request>", "fix the flaky test"},
		{"only the first line of a multi-line request", "<user-request>\nfix it\nand also this\n</user-request>", "fix it"},
		{"no wrapper at all", "just a plain prompt", "just a plain prompt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := titleSubject(c.wrapped); got != c.want {
				t.Errorf("titleSubject(%q) = %q, want %q", c.wrapped, got, c.want)
			}
		})
	}
}

func TestEnabledReadsEnvVar(t *testing.T) {
	t.Setenv("AGENTTIK_FAKE_PROVIDER", "")
	if Enabled() {
		t.Error("Enabled() with an empty env var, want false")
	}
	t.Setenv("AGENTTIK_FAKE_PROVIDER", "1")
	if !Enabled() {
		t.Error("Enabled() with AGENTTIK_FAKE_PROVIDER=1, want true")
	}
}
