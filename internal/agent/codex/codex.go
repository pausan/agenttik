// Package codex drives the locally installed Codex CLI, using the ChatGPT
// account already configured in ~/.codex.
//
// Status: stub. The command shape below is correct, but the JSONL event names
// codex exec emits differ between CLI versions, so the parser must be written
// against the installed version and pinned by a fixture test rather than
// guessed. Run returns agent.ErrNotImplemented until then.
package codex

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/pausan/agenttik/internal/agent"
)

var Binary = "codex"

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string        { return "codex" }
func (p *Provider) DisplayName() string { return "Codex" }

func (p *Provider) Models() []agent.Model {
	return []agent.Model{
		{ID: "gpt-5-codex", Label: "GPT-5 Codex"},
		{ID: "gpt-5", Label: "GPT-5"},
	}
}

func (p *Provider) Efforts() []string {
	return []string{"minimal", "low", "medium", "high"}
}

func (p *Provider) Available() error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("%s not found on PATH: %w", Binary, err)
	}
	return agent.ErrNotImplemented
}

func sandboxFlag(p agent.Permission) string {
	switch p.Valid() {
	case agent.PermissionPlan:
		return "read-only"
	case agent.PermissionFull:
		return "danger-full-access"
	default:
		return "workspace-write"
	}
}

// buildArgs is the invocation Run will use once event parsing lands.
func buildArgs(req agent.TurnRequest) []string {
	args := []string{"exec"}
	if req.ProviderSessionID != "" {
		args = append(args, "resume", req.ProviderSessionID)
	}
	args = append(args, "--json", "--cd", req.WorkDir)
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+req.Effort)
	}
	if req.Permission.Valid() == agent.PermissionFull {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else {
		args = append(args, "--sandbox", sandboxFlag(req.Permission))
	}
	return args
}

func (p *Provider) Run(context.Context, agent.TurnRequest) (<-chan agent.Event, error) {
	return nil, agent.ErrNotImplemented
}
