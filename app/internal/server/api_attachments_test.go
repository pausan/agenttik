package server

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachments(t *testing.T) {
	s, st := newTestServer(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var url string
	for range 2 {
		resp, err := s.app.Test(httptest.NewRequest("POST", "/api/attachments", bytes.NewReader(data.Bytes())))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode)
		}
		result := decode[map[string]string](t, resp)
		if url != "" && result["url"] != url {
			t.Fatal("duplicate image was not reused")
		}
		url = result["url"]
	}
	resp := do(t, s, "GET", url, nil)
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !bytes.Equal(got, data.Bytes()) {
		t.Fatal("image did not round trip")
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "attachments", filepath.Base(url))); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"", "not an image", strings.Repeat("x", 4*1024*1024+1)} {
		resp, err := s.app.Test(httptest.NewRequest("POST", "/api/attachments", strings.NewReader(body)))
		if err != nil {
			if len(body) > 4*1024*1024 && strings.Contains(err.Error(), "body size exceeds") {
				continue
			}
			t.Fatal(err)
		}
		if resp.StatusCode < 400 {
			t.Fatal("accepted invalid upload")
		}
	}
	if resp := do(t, s, "GET", "/api/attachments/not-an-image", nil); resp.StatusCode != 404 {
		t.Fatal(resp.StatusCode)
	}
}
