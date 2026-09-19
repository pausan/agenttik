package apiprovider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pausan/agenttik/app/internal/agent"
)

func TestConnectionLifecyclePrivateStorageAndIsolation(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer good-key" {
			w.WriteHeader(401)
			fmt.Fprint(w, "bad-key-must-not-escape")
			return
		}
		if r.URL.Path != "/models" {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"gpt-test","name":"GPT Test"},{"id":"text-embedding-test"},{"id":"gpt-test-codex"},{"id":"gpt-test-pro"}]}`)
	}))
	defer upstream.Close()
	definition := Definition{ID: "openai", Label: "OpenAI", BaseURL: upstream.URL, Protocol: "chat"}
	dir := t.TempDir()
	p := New(definition, dir)
	if p.Available() == nil || len(p.Models()) != 0 {
		t.Fatal("unconfigured provider enabled")
	}
	if err := p.Configure(context.Background(), "", true, false); err == nil {
		t.Fatal("accepted empty key")
	}
	if err := p.Configure(context.Background(), "good-key", true, false); err != nil {
		t.Fatal(err)
	}
	if p.Available() != nil || len(p.Models()) != 1 || !p.Connection().KeySet {
		t.Fatal(p.Models(), p.Connection())
	}
	if err := p.Configure(context.Background(), "bad-key-must-not-escape", true, false); err == nil || strings.Contains(err.Error(), "must-not-escape") {
		t.Fatal("unsafe failure", err)
	}
	if !p.Connection().Enabled || p.snapshot().Key != "good-key" {
		t.Fatal("failed update replaced working connection")
	}
	reloaded := New(definition, dir)
	if reloaded.Available() != nil || len(reloaded.Models()) != 1 {
		t.Fatal("connection not persisted")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(p.path())
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("key is not private", err)
		}
	}
	if err := p.Configure(context.Background(), "", false, false); err != nil {
		t.Fatal(err)
	}
	if p.Available() == nil || len(p.Models()) != 0 || !p.Connection().KeySet {
		t.Fatal("disable lost key or exposed models")
	}
	if _, err := p.Run(context.Background(), agent.TurnRequest{Model: "gpt-test"}); err == nil {
		t.Fatal("disabled provider ran")
	}
	if p.ForProfile(t.TempDir()).Available() == nil {
		t.Fatal("profile inherited key")
	}
	before := requests.Load()
	_ = p.Models()
	_ = p.Connection()
	if requests.Load() != before {
		t.Fatal("listing made network requests")
	}
	if err := p.Configure(context.Background(), "", true, false); err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveKey(); err != nil {
		t.Fatal(err)
	}
	if New(definition, dir).Connection().KeySet || len(p.Models()) != 0 {
		t.Fatal("key removal failed")
	}
	data, err := os.ReadFile(p.path())
	if err != nil || strings.Contains(string(data), "good-key") {
		t.Fatal("removed secret retained", err)
	}
}

func TestOpenRouterValidatesKeyAndFiltersToolModels(t *testing.T) {
	paths := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/key" {
			if r.Header.Get("Authorization") != "Bearer valid" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"data":{"label":"test"}}`)
			return
		}
		if r.URL.Path != "/models/user" {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"vendor/coder","name":"Coder","context_length":128000,"supported_parameters":["tools"],"architecture":{"output_modalities":["text"]}},{"id":"vendor/image","supported_parameters":["tools"],"architecture":{"output_modalities":["image"]}},{"id":"vendor/chat-only","supported_parameters":[]}]}`)
	}))
	defer upstream.Close()
	p := New(Definition{ID: "openrouter", Label: "OpenRouter", BaseURL: upstream.URL}, t.TempDir())
	if err := p.Configure(context.Background(), "invalid", true, false); err == nil {
		t.Fatal("public catalog accepted bad key")
	}
	if len(paths) != 1 || paths[0] != "/key" {
		t.Fatal(paths)
	}
	if err := p.Configure(context.Background(), "valid", true, false); err != nil {
		t.Fatal(err)
	}
	models := p.Models()
	if len(models) != 1 || models[0].ID != "vendor/coder" || models[0].ContextWindow != 128000 {
		t.Fatal(models)
	}
}

func TestAnthropicModelPagination(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "key" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect auth")
		}
		if r.URL.Query().Get("after_id") == "" {
			fmt.Fprint(w, `{"data":[{"id":"claude-first","display_name":"First"}],"has_more":true,"last_id":"claude-first"}`)
		} else {
			fmt.Fprint(w, `{"data":[{"id":"claude-second","display_name":"Second"}],"has_more":false}`)
		}
	}))
	defer upstream.Close()
	p := New(Definition{ID: "anthropic", Label: "Claude", BaseURL: upstream.URL, Protocol: "anthropic"}, t.TempDir())
	if err := p.Configure(context.Background(), "key", true, false); err != nil {
		t.Fatal(err)
	}
	if len(p.Models()) != 2 {
		t.Fatal(p.Models())
	}
}

func TestDiscoveryRejectsEmptyInvalidAndRedirectedCatalogs(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{{"empty", `{"data":[]}`, 200}, {"invalid", "not-json", 200}, {"unauthorized", "secret", 401}, {"redirect", "", 302}} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://example.com/")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer upstream.Close()
			p := New(Definition{ID: "openai", BaseURL: upstream.URL}, t.TempDir())
			if err := p.Configure(context.Background(), "key", true, false); err == nil {
				t.Fatal("accepted bad catalog")
			}
			if p.Connection().KeySet || p.Connection().Enabled {
				t.Fatal("failed key persisted")
			}
			if _, err := os.Stat(filepath.Join(p.dir, "connection.json")); !os.IsNotExist(err) {
				t.Fatal("failed setup wrote file", err)
			}
		})
	}
}

func TestModelCompatibilityFilters(t *testing.T) {
	for _, test := range []struct {
		provider, id string
		want         bool
	}{{"google", "gemini-test-pro", true}, {"google", "gemini-test-live", false}, {"google", "gemini-test-image", false}, {"groq", "llama-3.3-70b-versatile", true}, {"groq", "whisper-large-v3", false}, {"xai", "grok-code-fast-1", true}, {"xai", "grok-imagine-image", false}, {"deepseek", "deepseek-chat", true}, {"deepseek", "embedding", false}} {
		p := New(Definition{ID: test.provider}, t.TempDir())
		if got := p.supportsTools(catalogModel{ID: test.id}); got != test.want {
			t.Errorf("%s %s: %v", test.provider, test.id, got)
		}
	}
}
