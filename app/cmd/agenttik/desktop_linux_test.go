//go:build desktop && linux

package main

import (
	"testing"
)

func TestJSCSignalForGCValueDefault(t *testing.T) {
	t.Setenv("JSC_SIGNAL_FOR_GC", "")
	if got := jscSignalForGCValue(); got != jscSignalForGC {
		t.Fatalf("JSC signal=%d, want %d", got, jscSignalForGC)
	}
}

func TestJSCSignalForGCValueExplicit(t *testing.T) {
	const signal = "30"
	t.Setenv("JSC_SIGNAL_FOR_GC", signal)
	if got := jscSignalForGCValue(); got != 30 {
		t.Fatalf("JSC signal=%d, want explicit value 30", got)
	}
}
