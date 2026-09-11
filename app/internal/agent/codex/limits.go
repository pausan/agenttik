package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Codex reports its ChatGPT subscription allowance in the shape every
// provider uses, agent.RateLimit. Values come straight from Codex app-server;
// agenttik never sees the account credential that app-server uses.

type appServerResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type appServerRateLimits struct {
	RateLimits          *appServerRateLimit           `json:"rateLimits"`
	RateLimitsByLimitID map[string]appServerRateLimit `json:"rateLimitsByLimitId"`
}

type appServerRateLimit struct {
	LimitID     string                    `json:"limitId"`
	LimitName   string                    `json:"limitName"`
	PlanType    string                    `json:"planType"`
	Primary     *appServerRateLimitWindow `json:"primary"`
	Secondary   *appServerRateLimitWindow `json:"secondary"`
	ReachedType string                    `json:"rateLimitReachedType"`
}

type appServerRateLimitWindow struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins int64   `json:"windowDurationMins"`
	ResetsAt           int64   `json:"resetsAt"`
}

func publicRateLimit(limit appServerRateLimit) agent.RateLimit {
	window := func(raw *appServerRateLimitWindow) *agent.RateLimitWindow {
		if raw == nil {
			return nil
		}
		return &agent.RateLimitWindow{
			UsedPercent: raw.UsedPercent, WindowDurationMins: raw.WindowDurationMins, ResetsAt: raw.ResetsAt,
		}
	}
	return agent.RateLimit{
		LimitID: limit.LimitID, LimitName: limit.LimitName, PlanType: limit.PlanType,
		Primary: window(limit.Primary), Secondary: window(limit.Secondary), ReachedType: limit.ReachedType,
	}
}

// SubscriptionLimits asks the locally authenticated Codex app-server for its
// current ChatGPT subscription buckets, which makes this provider the Metered
// half of the contract. The short-lived helper has no UI or turn state, so it
// cannot alter the user's Codex conversations.
//
// stdin stays open until the answer arrives: app-server fetches the figures
// over the network and exits the moment its input closes, so a client that
// writes and closes gets no response at all.
func (p *Provider) SubscriptionLimits(ctx context.Context, home string) ([]agent.RateLimit, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, Binary, "app-server")
	// The allowance belongs to one subscription, so the ask carries the same
	// account the turns do.
	cmd.Env = agent.HomeEnv(HomeVar, home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("app-server stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s app-server: %w", Binary, err)
	}
	defer func() {
		cancel()
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	for _, message := range []any{
		map[string]any{"method": "initialize", "id": 0, "params": map[string]any{
			"clientInfo": map[string]string{"name": "agenttik", "title": "Agenttik", "version": "0.1"},
		}},
		map[string]any{"method": "initialized", "params": map[string]any{}},
		map[string]any{"method": "account/rateLimits/read", "id": 1},
	} {
		if err := enc.Encode(message); err != nil {
			return nil, fmt.Errorf("write app-server request: %w", err)
		}
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var response appServerResponse
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil || response.ID == nil || *response.ID != 1 {
			continue // initialization and update notifications are not this response
		}
		if response.Error != nil {
			return nil, fmt.Errorf("read subscription limits: %s", response.Error.Message)
		}
		var result appServerRateLimits
		if err := json.Unmarshal(response.Result, &result); err != nil {
			return nil, fmt.Errorf("decode subscription limits: %w", err)
		}
		if len(result.RateLimitsByLimitID) == 0 {
			if result.RateLimits == nil {
				return nil, nil
			}
			return []agent.RateLimit{publicRateLimit(*result.RateLimits)}, nil
		}

		limits := make([]agent.RateLimit, 0, len(result.RateLimitsByLimitID))
		if limit, ok := result.RateLimitsByLimitID["codex"]; ok {
			limits = append(limits, publicRateLimit(limit))
		}
		for id, limit := range result.RateLimitsByLimitID {
			if id != "codex" {
				limits = append(limits, publicRateLimit(limit))
			}
		}
		return limits, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read app-server response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read subscription limits: %w", err)
	}
	if text := strings.TrimSpace(stderr.String()); text != "" {
		return nil, fmt.Errorf("read subscription limits: %s", text)
	}
	return nil, fmt.Errorf("read subscription limits: app-server closed without a response")
}
