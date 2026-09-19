package apiprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/pausan/agenttik/app/internal/agent"
)

type catalogModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Context       int64    `json:"context_length"`
	MaxContext    int64    `json:"max_context_length"`
	ContextWindow int64    `json:"context_window"`
	Supported     []string `json:"supported_parameters"`
	Capabilities  *struct {
		Chat  bool `json:"completion_chat"`
		Tools bool `json:"function_calling"`
	} `json:"capabilities"`
	Architecture struct {
		Output []string `json:"output_modalities"`
	} `json:"architecture"`
}

func (p *Provider) get(ctx context.Context, key, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.definition.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "agenttik/1.0")
	if p.definition.Protocol == "anthropic" {
		req.Header.Del("Authorization")
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	response, err := p.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("model discovery timed out or was cancelled; try again")
		}
		return fmt.Errorf("cannot reach %s; check the connection and try again", p.definition.Label)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d; check the API key and model access", p.definition.Label, response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(target); err != nil {
		return fmt.Errorf("%s returned an invalid model catalog", p.definition.Label)
	}
	return nil
}

func (p *Provider) discover(ctx context.Context, key string) ([]agent.Model, error) {
	path := "/models"
	// OpenRouter's public catalog does not validate keys. Check this key first,
	// then use the account-filtered catalog so privacy/provider settings apply.
	if p.definition.ID == "openrouter" {
		var status json.RawMessage
		if err := p.get(ctx, key, "/key", &status); err != nil {
			return nil, err
		}
		path = "/models/user"
	}
	if p.definition.Protocol == "anthropic" {
		path += "?limit=100"
	}
	out := []agent.Model{}
	seen := map[string]bool{}
	for page := 0; page < 20; page++ {
		var body struct {
			Data    []catalogModel `json:"data"`
			HasMore bool           `json:"has_more"`
			LastID  string         `json:"last_id"`
		}
		if err := p.get(ctx, key, path, &body); err != nil {
			return nil, err
		}
		for _, m := range body.Data {
			m.ID = strings.TrimPrefix(m.ID, "models/")
			if m.ID == "" || seen[m.ID] || !p.supportsTools(m) {
				continue
			}
			seen[m.ID] = true
			label := m.DisplayName
			if label == "" {
				label = m.Name
			}
			if label == "" {
				label = m.ID
			}
			context := max(m.Context, m.MaxContext, m.ContextWindow)
			out = append(out, agent.Model{ID: m.ID, Label: label, ContextWindow: context})
		}
		if p.definition.Protocol != "anthropic" || !body.HasMore {
			break
		}
		if body.LastID == "" {
			return nil, fmt.Errorf("incomplete model catalog")
		}
		path = "/models?limit=100&after_id=" + url.QueryEscape(body.LastID)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no compatible models were returned for this key")
	}
	slices.SortFunc(out, func(a, b agent.Model) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return out, nil
}

func (p *Provider) supportsTools(m catalogModel) bool {
	id := strings.ToLower(m.ID)
	if p.definition.ID == "openrouter" {
		return slices.Contains(m.Supported, "tools") && (len(m.Architecture.Output) == 0 || slices.Contains(m.Architecture.Output, "text"))
	}
	if p.definition.ID == "mistral" {
		return m.Capabilities != nil && m.Capabilities.Chat && m.Capabilities.Tools
	}
	// Other catalogs lack tool capabilities. Restrict to conversational families
	// and exclude specialized endpoints rather than offering embeddings/media.
	for _, word := range []string{"embed", "whisper", "tts", "audio", "realtime", "image", "vision-preview", "moderation", "transcrib", "search", "deep-research", "computer-use", "instruct", "guard", "compound", "sora", "aqa"} {
		if strings.Contains(id, word) {
			return false
		}
	}
	switch p.definition.ID {
	case "openai":
		if strings.Contains(id, "codex") || strings.HasSuffix(id, "-pro") || strings.Contains(id, "-pro-") || strings.Contains(id, "chat-latest") {
			return false
		}
		return strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "o3") || strings.HasPrefix(id, "o4")
	case "anthropic":
		return strings.HasPrefix(id, "claude-")
	case "google":
		return strings.HasPrefix(id, "gemini-") && !strings.Contains(id, "robotics") && !strings.Contains(id, "live")
	case "xai":
		return strings.HasPrefix(id, "grok-")
	case "deepseek":
		return strings.HasPrefix(id, "deepseek-")
	case "groq":
		return strings.Contains(id, "llama") || strings.Contains(id, "qwen") || strings.Contains(id, "gpt-oss") || strings.Contains(id, "kimi")
	}
	return false
}
