package attachments

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	name := strings.Repeat("a", 64) + ".png"
	ref := "![Attached image](/api/attachments/" + name + ")"
	got := Resolve("Compare these\n"+ref+"\n"+ref, dir)
	if strings.Count(got, filepath.Join(dir, "attachments", name)) != 2 || !strings.HasPrefix(got, "Compare these\n") {
		t.Fatal(got)
	}
	for _, text := range []string{"ordinary prompt", "![Attached image](/api/attachments/../../secret)"} {
		if Resolve(text, dir) != text {
			t.Fatal("changed non-attachment text")
		}
	}
}
