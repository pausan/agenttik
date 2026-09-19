package opencode

import (
	"context"
	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/agent/direct"
	"path/filepath"
)

func (p *Provider) runDirect(ctx context.Context, req agent.TurnRequest) (<-chan agent.Event, error) {
	client := direct.Client{HTTP: p.client, Endpoint: p.endpoint, Key: p.key(req.AccountHome),
		HistoryDir: filepath.Join(p.home(req.AccountHome), "opencode", "agenttik-sessions"), SessionHeader: "x-opencode-session"}
	return client.Run(ctx, req)
}
