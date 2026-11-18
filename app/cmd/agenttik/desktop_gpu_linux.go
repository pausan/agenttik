//go:build desktop && linux

package main

/*
#cgo pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
#include <webkit2/webkit2.h>

int agenttik_gpu_check(void);

static unsigned webkit_version(void) {
	return webkit_get_major_version() * 10000 + webkit_get_minor_version() * 100 + webkit_get_micro_version();
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// The window draws on the GPU where that works: software painting costs a
// core's worth of CPU while a long reply streams and the transcript follows
// it. On some drivers an accelerated webview stays blank, so a child process
// checks first and the result is kept for this machine until its WebKit,
// kernel or graphics devices change. See specs/079-ui-performance.md.

const (
	gpuCheckArg = "--internal-gpu-check"
	// Bump to check every machine again after changing the check.
	gpuCheckRevision = 1
	gpuCheckTimeout  = 5 * time.Second
)

// The child is this executable; init runs it before anything else starts.
func init() {
	if len(os.Args) == 2 && os.Args[1] == gpuCheckArg {
		configureJSCSignalForGC()
		os.Exit(int(C.agenttik_gpu_check()))
	}
}

type gpuVerdict struct {
	Key    string `json:"key"`
	GPU    bool   `json:"gpu"`
	Reason string `json:"reason"`
}

// gpuPolicy picks the window's hardware acceleration policy. AGENTTIK_GPU=on
// or off skips the check.
func gpuPolicy() linux.WebviewGpuPolicy {
	switch strings.ToLower(os.Getenv("AGENTTIK_GPU")) {
	case "on", "1", "true":
		return linux.WebviewGpuPolicyAlways
	case "off", "0", "false":
		return linux.WebviewGpuPolicyNever
	}
	path := gpuVerdictPath()
	verdict := cachedGPUVerdict(path, gpuMachineKey(), checkGPU)
	if !verdict.GPU {
		log.Printf("desktop: drawing in software: %s (AGENTTIK_GPU=on forces the GPU)", verdict.Reason)
		return linux.WebviewGpuPolicyNever
	}
	return linux.WebviewGpuPolicyAlways
}

func gpuVerdictPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agenttik", "gpu-check.json")
}

// cachedGPUVerdict returns the verdict saved for key, or runs check and saves
// what it finds. Without a cache path it checks every time.
func cachedGPUVerdict(path, key string, check func() (bool, string)) gpuVerdict {
	if path != "" {
		var saved gpuVerdict
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &saved) == nil && saved.Key == key {
			return saved
		}
	}
	gpu, reason := check()
	verdict := gpuVerdict{Key: key, GPU: gpu, Reason: reason}
	if path != "" {
		data, _ := json.Marshal(verdict)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			_ = os.WriteFile(path, data, 0o600)
		}
	}
	return verdict
}

// checkGPU runs the check in a child process, so a driver that crashes or
// hangs costs only the child.
func checkGPU() (bool, string) {
	self, err := os.Executable()
	if err != nil {
		return false, "cannot find own executable: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), gpuCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, gpuCheckArg)
	out, err := cmd.Output()
	renderer := strings.TrimSpace(string(out))
	return gpuCheckResult(ctx.Err() != nil, cmd.ProcessState.ExitCode(), err, renderer)
}

func gpuCheckResult(timedOut bool, code int, err error, renderer string) (bool, string) {
	var exit *exec.ExitError
	switch {
	case timedOut:
		return false, "the GPU check did not finish in " + gpuCheckTimeout.String()
	case code == 0:
		return true, "GPU renderer " + strings.TrimSpace(renderer)
	case code == 1:
		return false, "an accelerated view drew nothing"
	case code == 2:
		return false, "only a software GL renderer (" + renderer + ")"
	case code == 3:
		return false, "no display for the GPU check"
	case errors.As(err, &exit):
		return false, fmt.Sprintf("the GPU check failed: %v", err)
	default:
		return false, fmt.Sprintf("the GPU check did not run: %v", err)
	}
}

// gpuMachineKey changes when what decides whether accelerated drawing works
// does: WebKit, the kernel and its graphics drivers, the GPUs present and the
// kind of display.
func gpuMachineKey() string {
	parts := []string{fmt.Sprintf("r%d", gpuCheckRevision), fmt.Sprintf("webkit %d", uint(C.webkit_version()))}
	if release, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		parts = append(parts, "kernel "+strings.TrimSpace(string(release)))
	}
	if nvidia, err := os.ReadFile("/sys/module/nvidia/version"); err == nil {
		parts = append(parts, "nvidia "+strings.TrimSpace(string(nvidia)))
	}
	nodes, _ := filepath.Glob("/sys/class/drm/renderD*")
	sort.Strings(nodes)
	for _, node := range nodes {
		vendor, _ := os.ReadFile(filepath.Join(node, "device", "vendor"))
		device, _ := os.ReadFile(filepath.Join(node, "device", "device"))
		driver, _ := filepath.EvalSymlinks(filepath.Join(node, "device", "driver"))
		parts = append(parts, fmt.Sprintf("%s %s:%s %s", filepath.Base(node),
			strings.TrimSpace(string(vendor)), strings.TrimSpace(string(device)), filepath.Base(driver)))
	}
	session := "x11"
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		session = "wayland"
	}
	return strings.Join(append(parts, session), "; ")
}
