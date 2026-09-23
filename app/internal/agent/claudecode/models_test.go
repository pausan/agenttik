package claudecode

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const modelMetadata = `{"type":"control_response","response":{"request_id":"models","response":{"models":[
 {"value":"default","resolvedModel":"claude-opus-4-8"},
 {"value":"opus[1m]","resolvedModel":"claude-opus-5-5[1m]"},
 {"value":"claude-fable-5[1m]","resolvedModel":"claude-fable-5"},
 {"value":"sonnet","resolvedModel":"claude-sonnet-5"},
 {"value":"haiku","resolvedModel":"claude-haiku-4-5-20251001"},
 {"value":"claude-opus-5","resolvedModel":"claude-opus-5"}
]}}}`

func TestParseModelLabels(t *testing.T) {
	// JSONL keeps each message on one line.
	data := "noise\n" + compactJSON(modelMetadata) + "\n"
	want := map[string]string{"opus": "Opus 5.5", "fable": "Fable 5", "sonnet": "Sonnet 5", "haiku": "Haiku 4.5"}
	if got := parseModelLabels(data); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	for _, data := range []string{`{}`, `{"type":"control_response","response":{"request_id":"models","response":{"models":[{"value":"opus","resolvedModel":"custom-model"}]}}}`, `{"type":"control_response","response":{"request_id":"other","response":{"models":[{"value":"opus","resolvedModel":"claude-opus-5"}]}}}`} {
		if got := parseModelLabels(data); len(got) != 0 {
			t.Fatalf("unexpected labels: %v", got)
		}
	}
}

func TestModelsLabelsCachedAndAliasesPreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncat >/dev/null\ncat <<'JSON'\n" + compactJSON(modelMetadata) + "\nJSON\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	old := Binary
	Binary = binary
	t.Cleanup(func() { Binary = old })
	p := New()
	models := p.Models()
	if models[1].ID != "opus" || models[1].Label != "Opus 5.5" {
		t.Fatalf("opus = %+v", models[1])
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if got := p.Models(); !reflect.DeepEqual(got, models) {
		t.Fatalf("cache = %+v", got)
	}
	p.cache.asked = time.Now().Add(-11 * time.Minute)
	if got := p.Models(); !reflect.DeepEqual(got, models) {
		t.Fatalf("failed refresh = %+v", got)
	}
	if got := New().Models()[1]; got.ID != "opus" || got.Label != "Opus (version unknown)" {
		t.Fatalf("fallback = %+v", got)
	}
}

func compactJSON(s string) string { return strings.ReplaceAll(s, "\n", "") }
