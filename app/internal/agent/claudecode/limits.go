package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/agent"
)

// Claude Code volunteers one bucket mid-turn — whichever is nearest its
// ceiling — so a subscription with room left in every window reports nothing
// at all, and the 5-hour and weekly bars never appear. Its own /usage command
// names every window, and print mode answers it locally: no model call, no
// cost, about two seconds. That makes this provider the Metered half of the
// contract as well as the remembering half. agenttik never sees the account
// credential; the CLI answers with its own, as Codex app-server does.

// usageLine is one rendered allowance in that report: a title, a floored
// percent, and a reset the CLI has already formatted for the local zone.
var usageLine = regexp.MustCompile(`^(.+?): (\d+)% used(?: · resets (.+))?$`)

// usageTitles maps the CLI's own headings onto the bucket ids it uses on a
// rate_limit_event, so a mid-turn reading refreshes the bar /usage drew
// instead of adding a second one for the same window.
var usageTitles = map[string]struct{ id, label string }{
	"Current session":            {"five_hour", "5-hour"},
	"Current week (all models)":  {"seven_day", "Weekly"},
	"Current week (Sonnet only)": {"seven_day_sonnet", "Weekly (Sonnet)"},
}

// usageBucket names one heading. Per-model weeklies come and go with the
// plans on offer — "Current week (Fable)" appeared without a release note —
// so an unlisted one is derived from its own title rather than dropped.
func usageBucket(title string) (id, label string) {
	if known, ok := usageTitles[title]; ok {
		return known.id, known.label
	}
	if model := strings.TrimSuffix(strings.TrimPrefix(title, "Current week ("), ")"); model != title {
		return "seven_day_" + slug(model), "Weekly (" + model + ")"
	}
	return slug(title), title
}

func slug(text string) string {
	return strings.ToLower(strings.ReplaceAll(text, " ", "_"))
}

// resetLayouts are what the CLI renders: the minutes vanish on the hour, and
// the year appears only when the reset falls outside the current one.
var resetLayouts = []string{"Jan 2, 2006, 3:04pm", "Jan 2, 2006, 3pm", "Jan 2, 3:04pm", "Jan 2, 3pm"}

// parseReset turns "Sep 15, 4pm (Europe/Madrid)" into Unix seconds. The zone
// is named in the text, and a missing year means the current one — the CLI
// prints the year precisely when it differs — so nothing here is guessed. An
// unreadable reset is 0, which the panel already shows as "not reported"
// rather than inventing a time.
func parseReset(text string) int64 {
	loc := time.Local
	if open := strings.LastIndex(text, "("); open > 0 && strings.HasSuffix(text, ")") {
		if zone, err := time.LoadLocation(text[open+1 : len(text)-1]); err == nil {
			loc = zone
		}
		text = strings.TrimSpace(text[:open])
	}
	for _, layout := range resetLayouts {
		at, err := time.ParseInLocation(layout, text, loc)
		if err != nil {
			continue
		}
		if at.Year() == 0 {
			at = at.AddDate(time.Now().In(loc).Year(), 0, 0)
		}
		return at.Unix()
	}
	return 0
}

// parseUsage reads the allowance lines out of /usage's own report. Everything
// else in it — the header, the behaviour breakdown — is not an allowance and
// is skipped, so a plan that meters nothing simply has no buckets.
func parseUsage(report string) []agent.RateLimit {
	var limits []agent.RateLimit
	for _, line := range strings.Split(report, "\n") {
		m := usageLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		percent, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		id, label := usageBucket(m[1])
		limits = append(limits, agent.RateLimit{
			LimitID:   id,
			LimitName: label,
			Primary: &agent.RateLimitWindow{
				Label: label, UsedPercent: percent, ResetsAt: parseReset(m[3]),
			},
		})
	}
	return limits
}

// SubscriptionLimits asks the signed-in CLI to report its own subscription
// windows. The flags are the ones a title request uses — no project settings
// or hooks, no tools, no session left behind — except that slash commands
// stay on, since /usage is the whole request.
func (p *Provider) SubscriptionLimits(ctx context.Context, home string) ([]agent.RateLimit, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, Binary, "-p", "/usage", "--output-format", "json",
		"--no-session-persistence", "--safe-mode", "--setting-sources", "user", "--tools", "")
	// The allowance belongs to one subscription, so the ask carries the same
	// account the turns do.
	cmd.Env = agent.HomeEnv(HomeVar, home)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read %s usage: %w", Binary, err)
	}
	var body struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		return nil, fmt.Errorf("decode %s usage: %w", Binary, err)
	}
	if body.IsError {
		return nil, fmt.Errorf("read %s usage: %s", Binary, strings.TrimSpace(body.Result))
	}
	return parseUsage(body.Result), nil
}
