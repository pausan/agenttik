package store

// Standard short-context USD per million tokens, verified 2026-09-21:
// https://developers.openai.com/api/docs/pricing
// Keep a versioned basis on each turn: these are API-equivalent estimates,
// not subscription charges. Unknown models must not inherit another price.
var codexPrices = map[string][4]float64{
	"gpt-6-astra":   {10, 1, 12.5, 50},
	"gpt-5.6-sol":   {4, .4, 5, 20},
	"gpt-5.6-terra": {2, .2, 2.5, 12},
	"gpt-5.6-luna":  {.2, .02, .25, 1.2},
	"gpt-5.3-codex": {1.75, .175, 0, 14},
}

func estimateTurnCost(provider, model string, t *Turn) (float64, string) {
	price, ok := codexPrices[model]
	if provider != "codex" || !ok || t.CostUSD != 0 {
		return 0, ""
	}
	// Codex input totals include cache reads (unlike Claude's counters).
	input, cached, writes, output := t.InputTokens, t.CacheReadTokens, t.CacheWriteTokens, t.OutputTokens
	if input < 0 || cached < 0 || writes < 0 || output < 0 || cached > input || writes > input-cached || (writes > 0 && price[2] == 0) {
		return 0, ""
	}
	if input == 0 && output == 0 {
		return 0, ""
	}
	cost := (float64(input-cached-writes)*price[0] + float64(cached)*price[1] + float64(writes)*price[2] + float64(output)*price[3]) / 1_000_000
	return cost, "openai-standard-short-context-2026-09-21"
}
