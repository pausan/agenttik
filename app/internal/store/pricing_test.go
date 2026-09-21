package store

import (
	"math"
	"testing"
)

func TestEstimateTurnCost(t *testing.T) {
	for _, tc := range []struct {
		name, provider, model string
		usage                 Turn
		want                  float64
		known                 bool
	}{
		{"cached input", "codex", "gpt-6-astra", Turn{InputTokens: 1000000, CacheReadTokens: 800000, OutputTokens: 100000}, 7.8, true},
		{"cache writes", "codex", "gpt-5.6-sol", Turn{InputTokens: 1000000, CacheReadTokens: 500000, CacheWriteTokens: 100000}, 2.3, true},
		{"terra", "codex", "gpt-5.6-terra", Turn{OutputTokens: 1000000}, 12, true},
		{"luna", "codex", "gpt-5.6-luna", Turn{OutputTokens: 1000000}, 1.2, true},
		{"older codex", "codex", "gpt-5.3-codex", Turn{InputTokens: 1000000}, 1.75, true},
		{"unknown", "codex", "gpt-future", Turn{InputTokens: 100}, 0, false},
		{"other provider", "custom", "gpt-6-astra", Turn{InputTokens: 100}, 0, false},
		{"reported wins", "codex", "gpt-6-astra", Turn{InputTokens: 100, CostUSD: 1}, 0, false},
		{"no usage", "codex", "gpt-6-astra", Turn{}, 0, false},
		{"invalid cache", "codex", "gpt-6-astra", Turn{InputTokens: 10, CacheReadTokens: 20}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost, basis := estimateTurnCost(tc.provider, tc.model, &tc.usage)
			if math.Abs(cost-tc.want) > 1e-10 || (basis != "") != tc.known {
				t.Fatalf("got %v %q", cost, basis)
			}
		})
	}
}

func TestCostEstimatePersistsAndAggregates(t *testing.T) {
	s := testStore(t)
	p, err := s.CreateProject("Pricing", "/tmp/pricing")
	must(t, err)
	must(t, s.CreateSession(&Session{ID: "pricing", ProjectID: p.ID, Provider: "codex", Model: "gpt-6-astra"}))
	turn, err := s.StartTurn("pricing", "gpt-6-astra", "high")
	must(t, err)
	// A subscription/model switch while running must not change pricing attribution.
	must(t, s.SetSessionModel("pricing", "claude", 0, "other", "", false))
	turn.InputTokens = 1000000
	turn.CacheReadTokens = 800000
	turn.OutputTokens = 100000
	must(t, s.FinishTurn(turn))
	must(t, s.FinishTurn(turn)) // replacement, not accumulation
	turns, err := s.ListTurns("pricing")
	must(t, err)
	if len(turns) != 1 || turns[0].EstimatedCostUSD != 7.8 || turns[0].CostEstimateBasis == "" || turns[0].CostUSD != 0 {
		t.Fatalf("turns: %+v", turns)
	}
	rows, err := s.Analytics(0, nowMillis()+1)
	must(t, err)
	if len(rows) != 1 || rows[0].EstimatedCostUSD != 7.8 || rows[0].EstimatedCostTurns != 1 || rows[0].CostTurns != 0 || rows[0].Model != "gpt-6-astra" {
		t.Fatalf("rows: %+v", rows)
	}
}
