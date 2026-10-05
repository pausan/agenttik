package server

import (
	"bytes"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestHexReadAndSave(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("hex", dir)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/projects/" + itoa(p.ID) + "/file?path=data.bin"
	abs := filepath.Join(dir, "data.bin")
	original := []byte{0, 65, 255, 128}
	if err := os.WriteFile(abs, original, 0o640); err != nil {
		t.Fatal(err)
	}
	loaded := decode[fileContent](t, do(t, s, "GET", endpoint, nil))
	if !loaded.Binary || loaded.Partial || loaded.Hex != "0041ff80" || loaded.Content != "" || len(loaded.Version) != 64 {
		t.Fatalf("hex response: %+v", loaded)
	}
	for _, body := range []any{
		map[string]any{"content": "text"},
		map[string]any{"hex": "0041ff80", "content": "text", "version": loaded.Version},
		map[string]any{"hex": "zz41ff80", "version": loaded.Version},
		map[string]any{"hex": "041ff80", "version": loaded.Version},
		map[string]any{"hex": "0041", "version": loaded.Version},
	} {
		resp := do(t, s, "PUT", endpoint, body)
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("accepted invalid write %v: %d", body, resp.StatusCode)
		}
		got, _ := os.ReadFile(abs)
		if !bytes.Equal(got, original) {
			t.Fatalf("invalid write changed bytes: %x", got)
		}
	}
	resp := do(t, s, "PUT", endpoint, map[string]any{"hex": "0041ff80"})
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("missing version: %d", resp.StatusCode)
	}
	saved := decode[struct {
		Size    int64  `json:"size"`
		Version string `json:"version"`
	}](t, do(t, s, "PUT", endpoint, map[string]any{"hex": "0042fe80", "version": loaded.Version}))
	got, _ := os.ReadFile(abs)
	info, _ := os.Stat(abs)
	if !bytes.Equal(got, []byte{0, 66, 254, 128}) || saved.Size != 4 || info.Mode().Perm() != 0o640 || saved.Version == loaded.Version {
		t.Fatalf("saved bytes %x; response %+v; mode %v", got, saved, info.Mode())
	}
	resp = do(t, s, "PUT", endpoint, map[string]any{"hex": "0043fe80", "version": loaded.Version})
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("stale version: %d", resp.StatusCode)
	}
	got, _ = os.ReadFile(abs)
	if !bytes.Equal(got, []byte{0, 66, 254, 128}) {
		t.Fatalf("stale save changed bytes: %x", got)
	}
}

func TestHexLimitsAndPaths(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("hex limits", dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + itoa(p.ID) + "/file?path="
	abs := filepath.Join(dir, "limit.bin")
	data := bytes.Repeat([]byte{128}, maxFileBytes)
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := decode[fileContent](t, do(t, s, "GET", base+"limit.bin", nil))
	if loaded.Partial || !loaded.Binary || len(loaded.Hex) != maxFileBytes*2 {
		t.Fatalf("limit read: size %d partial %v", len(loaded.Hex), loaded.Partial)
	}
	resp := do(t, s, "PUT", base+"limit.bin", map[string]any{"content": "text"})
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("text save accepted invalid UTF-8 binary: %d", resp.StatusCode)
	}
	data[0] = 255
	saved := decode[savedFile](t, do(t, s, "PUT", base+"limit.bin", map[string]any{"hex": hex.EncodeToString(data), "version": loaded.Version}))
	if saved.Size != maxFileBytes {
		t.Fatalf("limit save: %+v", saved)
	}
	if err := os.WriteFile(abs, append(data, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded = decode[fileContent](t, do(t, s, "GET", base+"limit.bin", nil))
	if !loaded.Partial || loaded.Size != maxFileBytes+1 || len(loaded.Hex) != maxFileBytes*2 {
		t.Fatalf("truncated read: size %d partial %v", loaded.Size, loaded.Partial)
	}
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, []byte{0, 255}, 0o600); err != nil {
		t.Fatal(err)
	}
	external := decode[fileContent](t, do(t, s, "GET", base+url.QueryEscape(outside), nil))
	if external.Hex != "00ff" {
		t.Fatalf("external preview: %+v", external)
	}
	for _, path := range []string{"limit.bin", outside, "../escape.bin"} {
		resp := do(t, s, "PUT", base+url.QueryEscape(path), map[string]any{"hex": loaded.Hex, "version": loaded.Version})
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("accepted forbidden save %s: %d", path, resp.StatusCode)
		}
	}
}

func TestExplicitHexForTextAndEmptyFiles(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("forced hex", dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/projects/" + itoa(p.ID) + "/file?path="
	for name, text := range map[string]string{"text.txt": "hello", "empty.bin": ""} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		plain := decode[fileContent](t, do(t, s, "GET", base+name, nil))
		if plain.Binary || plain.Content != text {
			t.Fatalf("text read: %+v", plain)
		}
		forced := decode[fileContent](t, do(t, s, "GET", base+name+"&format=hex", nil))
		if !forced.Binary || forced.Hex != hex.EncodeToString([]byte(text)) || len(forced.Version) != 64 {
			t.Fatalf("forced hex: %+v", forced)
		}
	}
}

func TestFileCapDoesNotMisclassifyUTF8(t *testing.T) {
	s, st := newTestServer(t)
	dir := t.TempDir()
	p, err := st.CreateProject("utf8 cap", dir)
	if err != nil {
		t.Fatal(err)
	}
	data := append(bytes.Repeat([]byte{'a'}, maxFileBytes-1), []byte("€")...)
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/projects/" + itoa(p.ID) + "/file?path=large.txt"
	got := decode[fileContent](t, do(t, s, "GET", endpoint, nil))
	if got.Binary || !got.Partial || len(got.Content) != maxFileBytes-1 {
		t.Fatalf("split rune: binary %v partial %v bytes %d", got.Binary, got.Partial, len(got.Content))
	}
	forced := decode[fileContent](t, do(t, s, "GET", endpoint+"&format=hex", nil))
	if !forced.Binary || len(forced.Hex) != maxFileBytes*2 {
		t.Fatalf("forced hex trimmed bytes: %d", len(forced.Hex))
	}
	data[maxFileBytes-1] = 255
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	got = decode[fileContent](t, do(t, s, "GET", endpoint, nil))
	if !got.Binary || len(got.Hex) != maxFileBytes*2 {
		t.Fatalf("invalid byte incorrectly trimmed: binary %v bytes %d", got.Binary, len(got.Hex))
	}
}
