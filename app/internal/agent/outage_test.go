package agent

import (
	"strconv"
	"testing"
	"time"
)

func TestOutageClassification(t *testing.T) {
	for _, tc := range []struct {
		failure  string
		away     bool
		capacity bool
	}{
		{"Selected model is at capacity. Please try a different model.", true, true},
		{"MODEL IS AT CAPACITY", true, true},
		{"service unavailable", true, false},
		{"rate limit reached", true, false},
		{"connection refused", true, false},
		{"unknown model", false, false},
		{"executable file not found", false, false},
		{"context capacity exceeded", false, false},
	} {
		t.Run(tc.failure, func(t *testing.T) {
			away, reset := Outage(tc.failure)
			if away != tc.away || !reset.IsZero() || AtCapacity(tc.failure) != tc.capacity {
				t.Fatalf("classification = %v, %v, capacity %v", away, reset, AtCapacity(tc.failure))
			}
		})
	}
	reset := time.Now().Add(time.Hour).Truncate(time.Second)
	away, got := Outage("usage limit reached|" + strconv.FormatInt(reset.Unix(), 10))
	if !away || !got.Equal(reset) {
		t.Fatalf("reset = %v, %v; want %v", away, got, reset)
	}
}
