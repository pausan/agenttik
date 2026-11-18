//go:build desktop && linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

func TestGPUPolicyOverride(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for value, want := range map[string]linux.WebviewGpuPolicy{
		"on": linux.WebviewGpuPolicyAlways, "1": linux.WebviewGpuPolicyAlways,
		"OFF": linux.WebviewGpuPolicyNever, "0": linux.WebviewGpuPolicyNever,
	} {
		t.Setenv("AGENTTIK_GPU", value)
		if got := gpuPolicy(); got != want {
			t.Errorf("AGENTTIK_GPU=%s: policy %v, want %v", value, got, want)
		}
	}
	if _, err := os.Stat(gpuVerdictPath()); !os.IsNotExist(err) {
		t.Errorf("an override wrote a verdict: %v", err)
	}
}

func TestCachedGPUVerdictChecksOncePerKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agenttik", "gpu-check.json")
	checks := 0
	check := func(gpu bool) func() (bool, string) {
		return func() (bool, string) { checks++; return gpu, "checked" }
	}
	if v := cachedGPUVerdict(path, "a", check(false)); v.GPU || checks != 1 {
		t.Fatalf("first verdict %+v after %d checks, want software after 1", v, checks)
	}
	if v := cachedGPUVerdict(path, "a", check(true)); v.GPU || checks != 1 {
		t.Fatalf("same key gave %+v after %d checks, want the saved software verdict", v, checks)
	}
	if v := cachedGPUVerdict(path, "b", check(true)); !v.GPU || checks != 2 {
		t.Fatalf("new key gave %+v after %d checks, want a fresh GPU verdict", v, checks)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := cachedGPUVerdict(path, "b", check(true)); !v.GPU || checks != 3 {
		t.Fatalf("a broken file gave %+v after %d checks, want a fresh check", v, checks)
	}
}

func TestGPUCheckResult(t *testing.T) {
	cases := []struct {
		timedOut bool
		code     int
		gpu      bool
		reason   string
	}{
		{false, 0, true, "GPU renderer AMD"},
		{false, 1, false, "drew nothing"},
		{false, 2, false, "software GL"},
		{false, 3, false, "no display"},
		{true, -1, false, "did not finish"},
		{false, -1, false, "did not run"},
	}
	for _, c := range cases {
		gpu, reason := gpuCheckResult(c.timedOut, c.code, nil, "AMD")
		if gpu != c.gpu || !strings.Contains(reason, c.reason) {
			t.Errorf("timedOut=%v code=%d: %v %q, want %v and %q", c.timedOut, c.code, gpu, reason, c.gpu, c.reason)
		}
	}
}

func TestGPUMachineKeyNamesWebKitAndSession(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	key := gpuMachineKey()
	if !strings.Contains(key, "webkit ") || !strings.HasSuffix(key, "x11") {
		t.Fatalf("key %q lacks the WebKit version or the session", key)
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if other := gpuMachineKey(); other == key {
		t.Fatalf("Wayland and X11 share key %q", key)
	}
}
