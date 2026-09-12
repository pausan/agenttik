package smartsearch

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestBekko(t *testing.T) {
	if os.Getenv("AGENTTIK_TEST_BEKKO") == "" {
		t.Skip("downloads real model and native runtimes")
	}
	dir := os.Getenv("AGENTTIK_TEST_BEKKO_DIR")
	if dir == "" {
		dir = filepath.Join(t.TempDir(), "smart-search")
	}
	m, err := loadModel(context.Background(), dir, func(s Status) { t.Log(s.Phase, int(s.Percent)) })
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if m != nil {
			_ = m.Close()
		}
	}()
	v, err := m.EmbedDocuments([]string{"Repair user login and authentication", "Change the background color", "autenticación de usuarios"})
	if err != nil {
		t.Fatal(err)
	}
	for _, vector := range v {
		if !validVector(vector) {
			t.Fatal("invalid vector")
		}
	}
	dot := func(a, b []float32) float64 {
		var sum float64
		for i := range a {
			sum += float64(a[i]) * float64(b[i])
		}
		return sum
	}
	login, color := dot(v[0], v[2]), dot(v[1], v[2])
	t.Logf("cross-language scores: login %.4f, color %.4f", login, color)
	if login < 0.3 || color >= 0.3 || login <= color {
		t.Fatal("cross-language retrieval failed")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m = nil
	t.Setenv("ONNXRUNTIME_DISABLE_DOWNLOAD", "1")
	originalTransport := http.DefaultTransport
	http.DefaultTransport = offlineTransport{}
	defer func() { http.DefaultTransport = originalTransport }()
	m, err = loadModel(context.Background(), dir, func(Status) {})
	if err != nil {
		t.Fatalf("cached offline reload: %v", err)
	}
	if _, err := m.EmbedDocuments([]string{"users cannot sign in"}); err != nil {
		t.Fatal(err)
	}
}

type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network disabled for cached model test")
}
