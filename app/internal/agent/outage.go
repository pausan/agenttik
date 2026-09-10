package agent

import (
	"strconv"
	"strings"
	"time"
)

// An outage is the provider being away, not the request being wrong: no
// internet under the CLI, a 5xx or an overload behind it, or a subscription
// allowance in front of it. The three are one case here because they end the
// same way — the same prompt, unchanged, works again later — and the caller
// answers all three by putting the prompt back in the queue.
//
// Only turn-level failures are read: the error a provider ends a turn with, or
// what a crashed CLI left on stderr. A tool result that happens to contain
// "connection refused" is the agent's own work and never reaches this.

// outageSigns are matched against the lowercased failure. Substrings rather
// than patterns: three CLIs wrap HTTP in their own wording and all of it moves
// between releases, so this reads for the vocabulary they share.
var outageSigns = []string{
	// The network under the CLI.
	"connection error", "connection refused", "connection reset", "connection closed",
	"econnrefused", "econnreset", "enotfound", "etimedout", "epipe", "eai_again",
	"socket hang up", "fetch failed", "network error", "getaddrinfo", "no such host",
	"temporary failure in name resolution", "dns", "tls handshake", "offline",

	// The service behind it.
	"overloaded", "server error", "bad gateway", "service unavailable",
	"gateway timeout", "upstream connect error", "high demand",
	"api error: 500", "api error: 502", "api error: 503", "api error: 504", "api error: 529",

	// The allowance in front of it.
	"usage limit reached", "rate limit", "rate_limit", "too many requests", "429",
	"quota", "credit balance", "out of credits",
}

// Outage reports whether a failure is the service being away, and when the
// provider said it would be back. A zero time means it did not say, and the
// caller picks its own delay.
func Outage(failure string) (bool, time.Time) {
	text := strings.ToLower(failure)
	for _, sign := range outageSigns {
		if strings.Contains(text, sign) {
			return true, resetAt(failure)
		}
	}
	return false, time.Time{}
}

// resetAt reads the reset instant a provider volunteers with its allowance
// failure. Claude Code appends it to the message as "…limit reached|<unix>",
// which turns a five-hour wait into one wait instead of a retry every minute
// for five hours. Anything else, including a reset already in the past, is no
// answer at all and leaves the caller to its own backoff.
func resetAt(failure string) time.Time {
	pipe := strings.LastIndex(failure, "|")
	if pipe < 0 {
		return time.Time{}
	}
	stamp, err := strconv.ParseInt(strings.TrimSpace(failure[pipe+1:]), 10, 64)
	if err != nil || stamp <= 0 {
		return time.Time{}
	}
	// Seconds or milliseconds: no plausible reset is 2286 or later, so the
	// magnitude says which unit was meant.
	at := time.Unix(stamp, 0)
	if stamp > 1e11 {
		at = time.UnixMilli(stamp)
	}
	if at.Before(time.Now()) {
		return time.Time{}
	}
	return at
}
