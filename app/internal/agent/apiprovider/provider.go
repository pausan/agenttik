// Package apiprovider connects shared API keys to the shared coding runtime.
package apiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/direct"
)

type Definition struct{ ID, Label, BaseURL, KeyURL, Protocol string }

var Catalog = []Definition{
	{"openrouter", "OpenRouter", "https://openrouter.ai/api/v1", "https://openrouter.ai/settings/keys", "chat"},
	{"openai", "OpenAI", "https://api.openai.com/v1", "https://platform.openai.com/api-keys", "chat"},
	{"anthropic", "Anthropic / Claude", "https://api.anthropic.com/v1", "https://platform.claude.com/settings/keys", "anthropic"},
	{"google", "Google Gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "https://aistudio.google.com/api-keys", "chat"},
	{"groq", "Groq", "https://api.groq.com/openai/v1", "https://console.groq.com/keys", "chat"},
	{"deepseek", "DeepSeek", "https://api.deepseek.com", "https://platform.deepseek.com/api_keys", "chat"},
	{"mistral", "Mistral", "https://api.mistral.ai/v1", "https://console.mistral.ai/api-keys", "chat"},
	{"xai", "xAI", "https://api.x.ai/v1", "https://console.x.ai/", "chat"},
}

type config struct {
	Key     string        `json:"key"`
	Enabled bool          `json:"enabled"`
	Models  []agent.Model `json:"models"`
}

// Connection is the write-only key's public status; it never contains a secret.
type Connection struct {
	Enabled bool   `json:"enabled"`
	KeySet  bool   `json:"key_set"`
	KeyURL  string `json:"key_url"`
}

type Provider struct {
	definition Definition
	shared     *Provider
	dir        string
	http       *http.Client
	update     sync.Mutex
	mu         sync.RWMutex
	config     config
	loadErr    error
}

func New(definition Definition, dataDir string) *Provider {
	p := &Provider{definition: definition, dir: filepath.Join(dataDir, "api-providers", definition.ID), http: &http.Client{
		Timeout:       5 * time.Minute,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
	data, err := os.ReadFile(p.path())
	if err == nil {
		if len(data) > 4<<20 || json.Unmarshal(data, &p.config) != nil {
			p.loadErr = fmt.Errorf("cannot read saved API configuration")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		p.loadErr = fmt.Errorf("cannot read saved API configuration")
	}
	return p
}
func All(dataDir string) []agent.Provider {
	out := make([]agent.Provider, 0, len(Catalog))
	for _, definition := range Catalog {
		out = append(out, New(definition, dataDir))
	}
	return out
}

// ForProfile shares the connection while keeping conversation files local.
func (p *Provider) ForProfile(dataDir string) *Provider {
	return &Provider{definition: p.definition, dir: filepath.Join(dataDir, "api-providers", p.definition.ID), http: p.http, shared: p}
}

// ImportConnection adopts a legacy profile connection when the shared one is
// empty. Preserve conflicting keys in the instance directory for recovery.
func (p *Provider) ImportConnection(dataDir string) error {
	legacyPath := filepath.Join(dataDir, "api-providers", p.definition.ID, "connection.json")
	data, err := os.ReadFile(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var legacy config
	if len(data) > 4<<20 || json.Unmarshal(data, &legacy) != nil {
		return fmt.Errorf("cannot read saved API configuration")
	}
	p.update.Lock()
	defer p.update.Unlock()
	if p.snapshot().Key == "" && legacy.Key != "" {
		if err := p.save(legacy); err != nil {
			return err
		}
	}
	backup := filepath.Join(p.dir, "legacy", filepath.Base(dataDir)+".json")
	if err := direct.PrivateWrite(backup, data); err != nil {
		return err
	}
	return os.Remove(legacyPath)
}
func (p *Provider) Name() string        { return "api-" + p.definition.ID }
func (p *Provider) DisplayName() string { return p.definition.Label + " API" }
func (p *Provider) Efforts() []string   { return nil }
func (p *Provider) path() string        { return filepath.Join(p.dir, "connection.json") }
func (p *Provider) snapshot() config {
	if p.shared != nil {
		return p.shared.snapshot()
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}
func (p *Provider) Connection() Connection {
	c := p.snapshot()
	return Connection{Enabled: c.Enabled && c.Key != "", KeySet: c.Key != "", KeyURL: p.definition.KeyURL}
}
func (p *Provider) Models() []agent.Model {
	c := p.snapshot()
	if !c.Enabled || c.Key == "" {
		return []agent.Model{}
	}
	return slices.Clone(c.Models)
}
func (p *Provider) Available() error {
	if p.shared != nil {
		return p.shared.Available()
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.loadErr != nil {
		return p.loadErr
	}
	if !p.config.Enabled || p.config.Key == "" {
		return fmt.Errorf("enable %s in Settings → Providers → API Providers", p.definition.Label)
	}
	return nil
}
func (p *Provider) Configure(ctx context.Context, key string, enabled, refresh bool) error {
	if p.shared != nil {
		return p.shared.Configure(ctx, key, enabled, refresh)
	}
	key = strings.TrimSpace(key)
	if len(key) > 8192 || strings.ContainsFunc(key, func(r rune) bool { return r < 33 || r > 126 }) {
		return fmt.Errorf("invalid API key")
	}
	p.update.Lock()
	defer p.update.Unlock()
	c := p.snapshot()
	if key != "" {
		c.Key = key
	}
	if enabled && c.Key == "" {
		return fmt.Errorf("enter an API key first")
	}
	if enabled && (key != "" || refresh || !c.Enabled || len(c.Models) == 0) {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		models, err := p.discover(ctx, c.Key)
		if err != nil {
			return err
		}
		c.Models = models
	}
	c.Enabled = enabled
	return p.save(c)
}
func (p *Provider) RemoveKey() error {
	if p.shared != nil {
		return p.shared.RemoveKey()
	}
	p.update.Lock()
	defer p.update.Unlock()
	return p.save(config{})
}
func (p *Provider) save(c config) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := direct.PrivateWrite(p.path(), data); err != nil {
		return fmt.Errorf("cannot save API connection")
	}
	p.mu.Lock()
	p.config, p.loadErr = c, nil
	p.mu.Unlock()
	return nil
}
func (p *Provider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	if err := p.Available(); err != nil {
		return nil, err
	}
	c := p.snapshot()
	if req.AccountHome != "" {
		return nil, fmt.Errorf("API connections do not use subscription accounts")
	}
	if req.Model == "" && len(c.Models) > 0 {
		req.Model = c.Models[0].ID
	}
	if !slices.ContainsFunc(c.Models, func(m agent.Model) bool { return m.ID == req.Model }) {
		return nil, fmt.Errorf("model is unavailable; refresh models in Settings → Providers → API Providers")
	}
	endpoint := p.definition.BaseURL + "/chat/completions"
	if p.definition.Protocol == "anthropic" {
		endpoint = p.definition.BaseURL + "/messages"
	}
	client := direct.Client{HTTP: p.http, Endpoint: endpoint, Protocol: p.definition.Protocol, Key: c.Key, HistoryDir: filepath.Join(p.dir, "sessions")}
	return client.Run(ctx, req)
}
