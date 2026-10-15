package server

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef"

// put sends one piece of an upload, the way the Tree does.
func put(t *testing.T, s *Server, base, path string, offset, size int, data []byte, extra string) int {
	t.Helper()
	q := url.Values{"path": {path}, "upload": {testToken}, "offset": {strconv.Itoa(offset)}, "size": {strconv.Itoa(size)}}
	req := httptest.NewRequest("PUT", base+"/upload?"+q.Encode()+extra, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func uploadProject(t *testing.T) (*Server, string, string) {
	t.Helper()
	s, st := newTestServer(t)
	root := t.TempDir()
	p, err := st.CreateProject("upload", root)
	if err != nil {
		t.Fatal(err)
	}
	return s, root, "/api/projects/" + itoa(p.ID)
}

func TestUploadWritesPiecesInOrder(t *testing.T) {
	s, root, base := uploadProject(t)
	data := []byte("hello, dropped world")
	if code := put(t, s, base, "in/new.txt", 0, len(data), data[:5], ""); code != 204 {
		t.Fatalf("first piece: %d", code)
	}
	// Nothing under the real name until the last piece lands.
	if _, err := os.Stat(filepath.Join(root, "in/new.txt")); !os.IsNotExist(err) {
		t.Fatalf("target exists early: %v", err)
	}
	if code := put(t, s, base, "in/new.txt", 3, len(data), data[3:], ""); code != 409 {
		t.Fatalf("overlapping piece: %d", code)
	}
	if code := put(t, s, base, "in/new.txt", 5, len(data), data[5:], ""); code != 200 {
		t.Fatalf("last piece: %d", code)
	}
	got, err := os.ReadFile(filepath.Join(root, "in/new.txt"))
	if err != nil || string(got) != string(data) {
		t.Fatalf("got %q %v", got, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "in")); len(entries) != 1 {
		t.Fatalf("partial file left behind: %v", entries)
	}
}

func TestUploadEmptyFileAndFolders(t *testing.T) {
	s, root, base := uploadProject(t)
	if code := put(t, s, base, "empty", 0, 0, nil, ""); code != 200 {
		t.Fatalf("empty file: %d", code)
	}
	if info, err := os.Stat(filepath.Join(root, "empty")); err != nil || info.Size() != 0 {
		t.Fatalf("empty file: %v %v", info, err)
	}
	for range 2 {
		resp := do(t, s, "PUT", base+"/upload?path=a/b&dir=1", nil)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("folder: %d", resp.StatusCode)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "a/b")); err != nil || !info.IsDir() {
		t.Fatalf("folder: %v", err)
	}
}

func TestUploadReplacesOnlyWhenAsked(t *testing.T) {
	s, root, base := uploadProject(t)
	target := filepath.Join(root, "keep.txt")
	os.WriteFile(target, []byte("old"), 0o644)
	if code := put(t, s, base, "keep.txt", 0, 3, []byte("new"), ""); code != 400 {
		t.Fatalf("unasked replace: %d", code)
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Fatalf("replaced unasked: %q", got)
	}
	if code := put(t, s, base, "keep.txt", 0, 3, []byte("new"), "&replace=1"); code != 200 {
		t.Fatalf("replace: %d", code)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Fatalf("not replaced: %q", got)
	}
	os.Mkdir(filepath.Join(root, "dir"), 0o755)
	if code := put(t, s, base, "dir", 0, 1, []byte("x"), "&replace=1"); code != 400 {
		t.Fatalf("replaced a folder: %d", code)
	}
}

func TestUploadRefusesBadInput(t *testing.T) {
	s, _, base := uploadProject(t)
	for _, c := range []struct {
		path, token, offset, size string
	}{
		{"../out", testToken, "0", "1"},
		{".git/config", testToken, "0", "1"},
		{"ok", "../../etc", "0", "1"},
		{"ok", testToken, "-1", "1"},
		{"ok", testToken, "0", "0"},
	} {
		q := url.Values{"path": {c.path}, "upload": {c.token}, "offset": {c.offset}, "size": {c.size}}
		req := httptest.NewRequest("PUT", base+"/upload?"+q.Encode(), strings.NewReader("x"))
		resp, err := s.app.Test(req, 5000)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("%+v: %d", c, resp.StatusCode)
		}
	}
}

func TestUploadCancelRemovesPartialFile(t *testing.T) {
	s, root, base := uploadProject(t)
	put(t, s, base, "big.bin", 0, 10, []byte("abc"), "")
	resp := do(t, s, "DELETE", base+"/upload?path=big.bin&upload="+testToken, nil)
	resp.Body.Close()
	if entries, _ := os.ReadDir(root); resp.StatusCode != 204 || len(entries) != 0 {
		t.Fatalf("cancel: %d %v", resp.StatusCode, entries)
	}
}

// A window on another machine's profile copies through this one: the pieces
// are forwarded like any other request and land on the remote's disk.
func TestUploadReachesRemoteMachine(t *testing.T) {
	remoteSrv, remoteStore := newTestServer(t)
	root := t.TempDir()
	p, err := remoteStore.CreateProject("remote", root)
	if err != nil {
		t.Fatal(err)
	}
	address := listen(t, remoteSrv)
	local, _ := newTestServer(t)
	if state := decode[remoteState](t, do(t, local, "POST", "/api/remotes", map[string]string{"address": address})); state.Status != "ok" {
		t.Fatalf("%+v", state)
	}
	data := bytes.Repeat([]byte("0123456789"), uploadChunkBytes/10+1)
	base := "/api/projects/" + itoa(p.ID)
	extra := "&remote=" + machineID(t, remoteStore)
	if code := put(t, local, base, "copied.bin", 0, len(data), data[:uploadChunkBytes], extra); code != 204 {
		t.Fatalf("first piece: %d", code)
	}
	if code := put(t, local, base, "copied.bin", uploadChunkBytes, len(data), data[uploadChunkBytes:], extra); code != 200 {
		t.Fatalf("last piece: %d", code)
	}
	if got, err := os.ReadFile(filepath.Join(root, "copied.bin")); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("remote copy: %d bytes %v", len(got), err)
	}
}
