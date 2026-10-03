//go:build desktop && linux

package main

import (
	"os"
	"testing"

	"github.com/pkg/browser"
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

func TestQuietBrowserOpenerSendsOutputToDevNull(t *testing.T) {
	stdout, stderr := browser.Stdout, browser.Stderr
	t.Cleanup(func() { browser.Stdout, browser.Stderr = stdout, stderr })
	quietBrowserOpener()
	for name, w := range map[string]any{"stdout": browser.Stdout, "stderr": browser.Stderr} {
		f, ok := w.(*os.File)
		if !ok || f.Name() != os.DevNull {
			t.Fatalf("browser %s=%v, want %s as an *os.File", name, w, os.DevNull)
		}
	}
}
