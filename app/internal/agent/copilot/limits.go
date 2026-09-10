package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

// quotaSnapshot is shared by the account RPC and assistant.usage events. The
// latter has a few extra fields; missing fields from the account response are
// harmless when decoded into this superset.
type quotaSnapshot struct {
	IsUnlimitedEntitlement           bool    `json:"isUnlimitedEntitlement"`
	EntitlementRequests              int64   `json:"entitlementRequests"`
	UsedRequests                     int64   `json:"usedRequests"`
	UsageAllowedWithExhaustedQuota   bool    `json:"usageAllowedWithExhaustedQuota"`
	Overage                          int64   `json:"overage"`
	OverageAllowedWithExhaustedQuota bool    `json:"overageAllowedWithExhaustedQuota"`
	RemainingPercentage              float64 `json:"remainingPercentage"`
	ResetDate                        string  `json:"resetDate"`
}

type rpcResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type accountQuotaResult struct {
	QuotaSnapshots map[string]quotaSnapshot `json:"quotaSnapshots"`
}

var quotaLabels = map[string]string{
	"premium_interactions": "Premium interactions",
	"chat":                 "Chat",
	"completions":          "Completions",
}

// quotaTimeout bounds the quota query. It sits on a UI refresh path, so a CLI
// that is slow to start is better reported as unreachable than waited on.
const quotaTimeout = 5 * time.Second

// SubscriptionLimits asks the headless Copilot CLI for the signed-in
// account's current quota. This is a read-only RPC and does not create a
// session or send a model request; the CLI supplies the existing login.
func (p *Provider) SubscriptionLimits(ctx context.Context) ([]agent.RateLimit, error) {
	result, err := serverQuery(ctx, quotaTimeout, "account.getQuota")
	if err != nil {
		return nil, fmt.Errorf("read subscription limits: %w", err)
	}
	var quota accountQuotaResult
	if err := json.Unmarshal(result, &quota); err != nil {
		return nil, fmt.Errorf("decode subscription limits: %w", err)
	}
	return publicQuotaLimits(quota.QuotaSnapshots), nil
}

// serverQuery starts the headless CLI, waits for it to answer a ping, then
// sends one read-only request and returns its result. Starting the CLI is
// most of the cost, so a caller that needs two answers is better served by
// two constants than by two queries.
func serverQuery(ctx context.Context, timeout time.Duration, method string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, Binary, "--headless", "--no-auto-update",
		"--log-level", "error", "--stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("copilot server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("copilot server stdout: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s server: %w", Binary, err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	reader := bufio.NewReaderSize(stdout, 64*1024)
	if err := writeRPC(stdin, 1, "ping"); err != nil {
		return nil, fmt.Errorf("write Copilot ping: %w", err)
	}
	if err := waitRPC(ctx, reader, 1); err != nil {
		return nil, fmt.Errorf("read Copilot server: %w", err)
	}
	if err := writeRPC(stdin, 2, method); err != nil {
		return nil, fmt.Errorf("write Copilot %s request: %w", method, err)
	}

	for {
		response, err := readRPC(reader)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// The CLI explains a refused query on stderr, and that reads
			// better than the EOF it closes the pipe with.
			if text := strings.TrimSpace(stderr.String()); text != "" {
				return nil, fmt.Errorf("%s", text)
			}
			return nil, err
		}
		if response.ID == nil || *response.ID != 2 {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("%s", response.Error.Message)
		}
		return response.Result, nil
	}
}

func writeRPC(w io.Writer, id int, method string) error {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  map[string]any{},
	})
	if err != nil {
		return err
	}
	message := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	_, err = io.WriteString(w, message)
	return err
}

func waitRPC(ctx context.Context, reader *bufio.Reader, id int) error {
	for {
		response, err := readRPC(reader)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if response.ID != nil && *response.ID == id {
			if response.Error != nil {
				return fmt.Errorf("RPC %s: %s", "ping", response.Error.Message)
			}
			return nil
		}
	}
}

func readRPC(reader *bufio.Reader) (rpcResponse, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return rpcResponse{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &contentLength); err != nil {
				return rpcResponse{}, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
	}
	if contentLength < 0 {
		return rpcResponse{}, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return rpcResponse{}, err
	}
	var response rpcResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return rpcResponse{}, err
	}
	return response, nil
}

func publicQuotaLimits(snapshots map[string]quotaSnapshot) []agent.RateLimit {
	ids := make([]string, 0, len(snapshots))
	for id, snapshot := range snapshots {
		if snapshot.EntitlementRequests == 0 && snapshot.UsedRequests == 0 && snapshot.Overage == 0 && !snapshot.IsUnlimitedEntitlement {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i] == "premium_interactions" {
			return true
		}
		if ids[j] == "premium_interactions" {
			return false
		}
		return ids[i] < ids[j]
	})

	limits := make([]agent.RateLimit, 0, len(ids))
	for _, id := range ids {
		snapshot := snapshots[id]
		label := quotaLabel(id)
		limits = append(limits, agent.RateLimit{
			LimitID:     id,
			LimitName:   label,
			ReachedType: quotaReachedType(snapshot),
			Primary: &agent.RateLimitWindow{
				Label:       label,
				UsedPercent: usedPercent(snapshot.RemainingPercentage),
				ResetsAt:    resetDate(snapshot.ResetDate),
			},
		})
	}
	return limits
}

func quotaLabel(id string) string {
	if label := quotaLabels[id]; label != "" {
		return label
	}
	parts := strings.Split(id, "_")
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func usedPercent(remaining float64) float64 {
	used := 100 - remaining
	if used < 0 {
		return 0
	}
	if used > 100 {
		return 100
	}
	return used
}

func resetDate(text string) int64 {
	if text == "" {
		return 0
	}
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return 0
	}
	return at.Unix()
}

func quotaReachedType(snapshot quotaSnapshot) string {
	if snapshot.Overage > 0 {
		return "using overage"
	}
	if snapshot.EntitlementRequests > 0 && snapshot.UsedRequests >= snapshot.EntitlementRequests &&
		!snapshot.OverageAllowedWithExhaustedQuota {
		return "quota exhausted"
	}
	return ""
}
