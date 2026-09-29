package server

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestFileRefFallback(t *testing.T) {
	s, st := newTestServer(t)
	root := t.TempDir()
	outside := t.TempDir()
	p, err := st.CreateProject("links", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"direct.txt", "a/nested/duplicate.txt", "b/duplicate.txt", "hint/chosen.txt", "hint/sub/combined.txt", "docs/near.txt", "node_modules/hidden.txt"} {
		abs := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	external := filepath.Join(outside, "external.txt")
	if err := os.WriteFile(external, []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, target string
		hints              []string
		want               string
		fail               bool
	}{
		{name: "direct wins", path: "direct.txt", hints: []string{"hint"}},
		{name: "absolute direct", path: external},
		{name: "folder hint", path: "chosen.txt", hints: []string{"hint"}, want: "hint/chosen.txt"},
		{name: "combined hint", path: "sub/combined.txt", target: "sub/combined.txt", hints: []string{"hint"}, want: "hint/sub/combined.txt"},
		{name: "file parent hint", path: "combined.txt", hints: []string{"hint/chosen.txt"}, want: "hint/sub/combined.txt"},
		{name: "hint before recursive", path: "duplicate.txt", hints: []string{"b"}, want: "b/duplicate.txt"},
		{name: "first recursive match", path: "duplicate.txt", want: "a/nested/duplicate.txt"},
		{name: "stale absolute", path: filepath.Join(outside, "old", "near.txt"), want: "docs/near.txt"},
		{name: "external hint", path: "external.txt", hints: []string{outside}, want: external},
		{name: "missing", path: "absent.txt", fail: true},
		{name: "skip dependencies", path: "hidden.txt", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hints, _ := json.Marshal(tc.hints)
			query := url.Values{"path": {tc.path}, "locate": {"1"}, "target": {tc.target}, "hints": {string(hints)}}
			resp := do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/file-info?"+query.Encode(), nil)
			if tc.fail {
				defer resp.Body.Close()
				if resp.StatusCode != 400 {
					t.Fatalf("status = %d", resp.StatusCode)
				}
				return
			}
			got := decode[fileInfo](t, resp)
			if got.Path != tc.want {
				t.Fatalf("path = %q, want %q", got.Path, tc.want)
			}
		})
	}
	// Ordinary metadata requests do not search or silently substitute files.
	resp := do(t, s, "GET", "/api/projects/"+itoa(p.ID)+"/file-info?path=duplicate.txt", nil)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
