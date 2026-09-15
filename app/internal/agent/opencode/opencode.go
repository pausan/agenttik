// Package opencode runs OpenCode Go through its CLI or documented coding-agent API.
package opencode

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

var Binary = "opencode"

type Provider struct {
	client   *http.Client
	endpoint string
}

func New() *Provider {
	return &Provider{client: &http.Client{Timeout: 5 * time.Minute}, endpoint: "https://opencode.ai/zen/go/v1/chat/completions"}
}
func (p *Provider) Name() string        { return "opencode" }
func (p *Provider) DisplayName() string { return "OpenCode Go" }
func (p *Provider) Available() error    { return nil }
func (p *Provider) Efforts() []string   { return nil }

// Only models with the documented Chat Completions endpoint are offered, so
// every choice works with either execution mode.
func (p *Provider) Models() []agent.Model {
	return []agent.Model{
		{ID: "glm-5.3-flash", Label: "GLM-5.3-Flash"}, {ID: "glm-5.3", Label: "GLM-5.3"},
		{ID: "glm-5.2", Label: "GLM-5.2"}, {ID: "glm-5.1", Label: "GLM-5.1"},
		{ID: "kimi-k3", Label: "Kimi K3"}, {ID: "kimi-k2.7-code", Label: "Kimi K2.7 Code"},
		{ID: "kimi-k2.6", Label: "Kimi K2.6"}, {ID: "longcat-2.0", Label: "LongCat-2.0"},
		{ID: "deepseek-v4.1-flash", Label: "DeepSeek V4.1 Flash"},
		{ID: "deepseek-v4-pro", Label: "DeepSeek V4 Pro"}, {ID: "deepseek-v4-flash", Label: "DeepSeek V4 Flash"},
		{ID: "mimo-v2.5", Label: "MiMo-V2.5"}, {ID: "mimo-v2.5-pro", Label: "MiMo-V2.5-Pro"},
		{ID: "hy4-preview", Label: "Hy4 preview"}, {ID: "hy3", Label: "Hy3"},
	}
}
func (p *Provider) SmallModel() (string, string) { return "glm-5.3-flash", "" }
func (p *Provider) useCLI(req agent.TurnRequest) bool {
	mode := p.ConnectionMode(req.AccountHome)
	if mode == "direct" {
		return false
	}
	if mode == "cli" {
		return true
	}
	// Keep an existing conversation on its original backend after PATH changes.
	if strings.HasPrefix(req.ProviderSessionID, "direct-") {
		return false
	}
	if strings.HasPrefix(req.ProviderSessionID, "ses_") {
		return true
	}
	_, err := exec.LookPath(Binary)
	return err == nil
}
func (p *Provider) Run(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	if err := req.CheckWorkDir(); err != nil {
		return nil, err
	}
	if req.Model == "" {
		req.Model, _ = p.SmallModel()
	}
	valid := false
	for _, model := range p.Models() {
		if model.ID == req.Model {
			valid = true
		}
	}
	if !valid {
		return nil, fmt.Errorf("unsupported OpenCode Go model %q", req.Model)
	}
	if !p.AccountStatus(req.AccountHome).SignedIn {
		return nil, fmt.Errorf("sign in to OpenCode Go in Settings → Subscriptions")
	}
	if p.useCLI(req) {
		return p.runCLI(ctx, req)
	}
	return p.runDirect(ctx, req)
}
