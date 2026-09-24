package codex

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

func modelFixture(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	binary := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	old := Binary
	Binary = binary
	t.Cleanup(func() { Binary = old })
	return binary
}

const modelHandshake = `read -r request
case "$request" in *'"method":"initialize"'*) ;; *) exit 1;; esac
printf '%s\n' '{"id":0,"result":{}}'
read -r request
case "$request" in *'"method":"initialized"'*) ;; *) exit 1;; esac
read -r request
case "$request" in *'"method":"model/list"'*) ;; *) exit 1;; esac
case "$request" in *'"cursor":""'*) exit 1;; esac
`

func TestModelsDiscoveryRefreshAndFallback(t *testing.T) {
	binary := modelFixture(t, modelHandshake+`
printf '%s\n' 'noise' '{"method":"notice"}' '{"id":1,"result":{"data":[{"id":"picker-id","model":"gpt-6-sol","displayName":"GPT-6 Sol","supportedReasoningEfforts":[{"reasoningEffort":"high"},{"reasoningEffort":"ultra"}]},{"model":"hidden","hidden":true},{}],"nextCursor":"page2"}}'
read -r request
case "$request" in *'"cursor":"page2"'*) ;; *) exit 1;; esac
printf '%s\n' '{"id":2,"result":{"data":[{"model":"gpt-6-sol"},{"model":"future-model"}],"nextCursor":null}}'
read -r request
`)
	p := New()
	want := []agent.Model{{ID: "gpt-6-sol", Label: "GPT-6 Sol", Efforts: []string{"high", "ultra"}}, {ID: "future-model", Label: "future-model"}}
	if got := p.Models(); !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %+v", got)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if got := p.Models(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cache = %+v", got)
	}
	p.cache.asked = time.Now().Add(-modelsTTL)
	if got := p.Models(); !reflect.DeepEqual(got, want) {
		t.Fatalf("failed refresh = %+v", got)
	}
	fresh := New()
	if got := fresh.Models(); !reflect.DeepEqual(got, fallbackModels()) {
		t.Fatalf("fallback = %+v", got)
	}
	// A new catalog replaces the old one on expiry, without restarting the app.
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+modelHandshake+`printf '%s\n' '{"id":1,"result":{"data":[{"model":"next-release"}]}}'
read -r request
`), 0700); err != nil {
		t.Fatal(err)
	}
	if got := fresh.Models(); !reflect.DeepEqual(got, fallbackModels()) {
		t.Fatal("failed discovery was not cached")
	}
	p.cache.asked = time.Now().Add(-modelsTTL)
	if got := p.Models(); len(got) != 1 || got[0].ID != "next-release" {
		t.Fatalf("refresh = %+v", got)
	}
}

func TestModelsDiscoveryErrors(t *testing.T) {
	for _, response := range []string{
		`{"id":1,"error":{"message":"unavailable"}}`,
		`{"id":1,"result":{"data":"invalid"}}`,
		`{"id":1,"result":{"data":[]}}`,
		`not json`,
	} {
		t.Run(response, func(t *testing.T) {
			modelFixture(t, modelHandshake+"printf '%s\\n' '"+response+"'\n")
			if got := New().Models(); !reflect.DeepEqual(got, fallbackModels()) {
				t.Fatalf("fallback = %+v", got)
			}
		})
	}
}

func TestModelsDiscoveryCancellation(t *testing.T) {
	modelFixture(t, "read -r request\nread -r request\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := fetchModels(ctx); err == nil {
		t.Fatal("expected timeout")
	}
}
