//go:build !windows

package update

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	if manifest := os.Getenv("AGENTTIK_TEST_UPDATE_PLAN"); manifest != "" {
		if err := Helper(manifest); err != nil {
			t.Fatal(err)
		}
	}
}
func TestHelperWaitsForExit(t *testing.T) {
	for _, scenario := range []string{"install", "cancel", "changed"} {
		t.Run(scenario, func(t *testing.T) { testHelperExit(t, scenario) })
	}
}
func testHelperExit(t *testing.T, scenario string) {
	dir := t.TempDir()
	target := filepath.Join(dir, "agenttik")
	source := filepath.Join(dir, "new")
	manifest := filepath.Join(dir, "plan.json")
	os.WriteFile(target, []byte("old"), 0755)
	os.WriteFile(source, []byte("new"), 0755)
	parent := exec.Command("sleep", "30")
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer parent.Process.Kill()
	plan := Plan{Parent: parent.Process.Pid, Archive: source, Target: target, Digest: release("v2.0.0", "linux_amd64").Assets[0].Digest}
	data, _ := json.Marshal(plan)
	os.WriteFile(manifest, data, 0600)
	helper := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	helper.Env = append(os.Environ(), "AGENTTIK_TEST_UPDATE_PLAN="+manifest)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer helper.Process.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(manifest + ".ready"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(manifest + ".error")
			t.Fatalf("helper not ready: %s", b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, _ = os.ReadFile(target)
	if string(data) != "old" {
		t.Fatal("installed before parent exit")
	}
	if scenario == "cancel" {
		os.WriteFile(manifest+".cancel", []byte("cancelled"), 0600)
	}
	if scenario == "changed" {
		other := filepath.Join(dir, "other")
		os.WriteFile(other, []byte("other"), 0755)
		if err := os.Rename(other, target); err != nil {
			t.Fatal(err)
		}
	}
	parent.Process.Kill()
	parent.Wait()
	if err := helper.Wait(); (err != nil) != (scenario != "install") {
		b, _ := os.ReadFile(manifest + ".error")
		t.Fatalf("helper: %v: %s", err, b)
	}
	data, _ = os.ReadFile(target)
	want := "new"
	if scenario == "cancel" {
		want = "old"
	}
	if scenario == "changed" {
		want = "other"
	}
	if string(data) != want {
		t.Fatalf("not installed: %s", data)
	}
	suffix := ".done"
	if scenario != "install" {
		suffix = ".error"
	}
	if _, err := os.Stat(manifest + suffix); err != nil {
		t.Fatal("missing completion marker")
	}
}
