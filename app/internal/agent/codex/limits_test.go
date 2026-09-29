package codex

import (
	"encoding/json"
	"testing"
)

func TestPublicRateLimitsOnlyShowsCodexAllowance(t *testing.T) {
	var result appServerRateLimits
	if err := json.Unmarshal([]byte(`{
		"rateLimitsByLimitId": {
			"codex": {"limitId":"codex","primary":{"usedPercent":25,"windowDurationMins":10080}},
			"base_model_inference": {"limitId":"base_model_inference","limitName":"gpt-reserve","primary":{"usedPercent":0,"windowDurationMins":10080}}
		}
	}`), &result); err != nil {
		t.Fatal(err)
	}
	limits := publicRateLimits(result)
	if len(limits) != 1 || limits[0].LimitID != "codex" || limits[0].Primary.UsedPercent != 25 {
		t.Fatalf("limits = %+v, want only the Codex allowance", limits)
	}
}

func TestPublicRateLimitsAcceptsLegacyReading(t *testing.T) {
	var result appServerRateLimits
	if err := json.Unmarshal([]byte(`{"rateLimits":{"limitId":"codex","primary":{"usedPercent":7}}}`), &result); err != nil {
		t.Fatal(err)
	}
	limits := publicRateLimits(result)
	if len(limits) != 1 || limits[0].Primary.UsedPercent != 7 {
		t.Fatalf("limits = %+v, want the legacy Codex allowance", limits)
	}
}
