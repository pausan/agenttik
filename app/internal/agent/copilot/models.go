package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

// modelsTimeout bounds the models.list query. It is longer than the quota
// query because nothing waits on it interactively: the list is fetched at
// startup, and a cold CLI start plus the round trip costs about two seconds.
const modelsTimeout = 10 * time.Second

// modelsTTL is how long an answer is served before the CLI is asked again.
// Models appear and entitlements change on the account, not on the keystroke.
const modelsTTL = 10 * time.Minute

// fallbackModels stand in until the CLI answers, and for good when it cannot
// be asked: no CLI on PATH, or one that will not report the account. This is
// a snapshot of what the CLI advertised when it was written, so it goes stale
// exactly the way the live query exists to avoid. It is here only so the app
// always has a model to name, never as the list of record.
var fallbackModels = []agent.Model{
	{ID: "gpt-5.3-codex", Label: "GPT-5.3-Codex", ContextWindow: 400_000,
		Efforts: []string{"low", "medium", "high", "xhigh"}},
	{ID: "gpt-4.1", Label: "GPT-4.1", ContextWindow: 128_000},
	{ID: "claude-opus-5", Label: "Claude Opus 5", ContextWindow: 264_000,
		Efforts: []string{"medium"}},
	{ID: "claude-sonnet-5", Label: "Claude Sonnet 5", ContextWindow: 264_000,
		Efforts: []string{"medium"}},
}

// fallbackTitle is the title model to use while the list above is standing in.
// gpt-4.1 is included at no cost and supports no reasoning effort.
const fallbackTitle, fallbackTitleEffort = "gpt-4.1", ""

// modelList is the models.list result. The CLI reports far more per model
// than this; what is decoded is what the app can show or act on.
type modelList struct {
	Models []modelInfo `json:"models"`
}

type modelInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Capabilities struct {
		Limits struct {
			MaxContextWindowTokens int64 `json:"max_context_window_tokens"`
		} `json:"limits"`
	} `json:"capabilities"`
	Billing *struct {
		TokenPrices *struct {
			InputPrice float64 `json:"input_price"`
		} `json:"token_prices"`
	} `json:"billing"`
	// SupportedReasoningEfforts is the levels this model accepts, which is
	// narrower than the levels the model is capable of: the CLI restricts
	// what it will pass on. Absent means the model takes no effort at all.
	SupportedReasoningEfforts []string `json:"supportedReasoningEfforts"`
	DefaultReasoningEffort    string   `json:"defaultReasoningEffort"`
}

// modelCache holds what the CLI last said, and when it was asked.
type modelCache struct {
	mu     sync.Mutex
	models []agent.Model
	title  string
	effort string
	asked  time.Time
}

// Models are the models the signed-in Copilot account can use, as the CLI
// itself reports them. It is the only authority worth asking: which models
// exist, and which the account is entitled to, both change without a release
// here, and the list it answers with is already the one it would show in its
// own picker.
//
// Asking costs a CLI start, so the answer is cached for modelsTTL. The
// fallback list stands in until the first answer arrives.
func (p *Provider) Models() []agent.Model {
	models, _, _ := p.models()
	return models
}

// Efforts is the widest set any Copilot model takes, and only a floor for a
// model the CLI has not described: each model carries its own levels.
func (p *Provider) Efforts() []string {
	return []string{"low", "medium", "high", "xhigh"}
}

// SmallModel is the cheapest model the account can use, which is all a
// one-shot request needs. Taking it from the live list keeps it from naming a
// model the CLI has since dropped.
func (p *Provider) SmallModel() (model, effort string) {
	_, model, effort = p.models()
	return model, effort
}

func (p *Provider) models() (models []agent.Model, title, effort string) {
	c := &p.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.asked) < modelsTTL && len(c.models) > 0 {
		return c.models, c.title, c.effort
	}
	// Stamped before the ask, not after, so a CLI that is missing or logged
	// out is retried on the same interval rather than on every request. The
	// lock is held across it so a burst of requests starts one CLI, not one
	// each, and the rest read what it brought back.
	c.asked = time.Now()
	if p.Available() == nil {
		if live, err := fetchModels(context.Background()); err == nil && len(live.models) > 0 {
			c.models, c.title, c.effort = live.models, live.title, live.effort
			return c.models, c.title, c.effort
		}
	}
	if len(c.models) == 0 {
		c.models, c.title, c.effort = fallbackModels, fallbackTitle, fallbackTitleEffort
	}
	return c.models, c.title, c.effort
}

// modelChoice is a converted list together with the title model picked out of
// it, so the price each pick rests on does not have to be carried any further.
type modelChoice struct {
	models []agent.Model
	title  string
	effort string
}

func fetchModels(ctx context.Context) (modelChoice, error) {
	result, err := serverQuery(ctx, modelsTimeout, "models.list", "")
	if err != nil {
		return modelChoice{}, fmt.Errorf("list Copilot models: %w", err)
	}
	var list modelList
	if err := json.Unmarshal(result, &list); err != nil {
		return modelChoice{}, fmt.Errorf("decode Copilot models: %w", err)
	}
	return convertModels(list.Models), nil
}

// convertModels keeps the CLI's order. It is the order the CLI offers them
// in, and the first entry is what a new session starts on.
func convertModels(models []modelInfo) modelChoice {
	choice := modelChoice{models: make([]agent.Model, 0, len(models))}
	cheapest := math.Inf(1)
	for _, m := range models {
		if m.ID == "" {
			continue
		}
		label := m.Name
		if label == "" {
			label = m.ID
		}
		choice.models = append(choice.models, agent.Model{
			ID:            m.ID,
			Label:         label,
			Efforts:       m.SupportedReasoningEfforts,
			ContextWindow: m.Capabilities.Limits.MaxContextWindowTokens,
		})
		if price, ok := m.inputPrice(); ok && price < cheapest {
			cheapest, choice.title, choice.effort = price, m.ID, m.titleEffort()
		}
	}
	return choice
}

// inputPrice is what the account is charged per batch of input tokens. A
// model the CLI quotes no price for is not a candidate for anything, so an
// unpriced model reports no price rather than a free one.
func (m modelInfo) inputPrice() (float64, bool) {
	if m.Billing == nil || m.Billing.TokenPrices == nil {
		return 0, false
	}
	return m.Billing.TokenPrices.InputPrice, true
}

// titleEffort is the least the model will think for, since naming a session
// needs no reasoning. A model that takes no effort is given none.
func (m modelInfo) titleEffort() string {
	for _, effort := range m.SupportedReasoningEfforts {
		if effort == "low" {
			return "low"
		}
	}
	return m.DefaultReasoningEffort
}
