package smartsearch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
)

type fakeModel struct {
	texts  []string
	closed bool
	fail   bool
}

func (m *fakeModel) EmbedDocuments(texts []string) ([][]float32, error) {
	if m.fail {
		return nil, errors.New("inference failed")
	}
	m.texts = append(m.texts, texts...)
	v := make([]float32, 384)
	v[0] = 1
	return [][]float32{v}, nil
}
func (m *fakeModel) Close() error { m.closed = true; return nil }

func waitIndex(t *testing.T, s *Service) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		busy, status := s.busy, s.status
		s.mu.Unlock()
		if !busy {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("index did not finish")
	return Status{}
}

func TestIndexCacheUpdatesDeletionAndQueries(t *testing.T) {
	tasks := []store.Session{{ID: "a", Title: "Login", Prompt: "Fix users", Summary: "Done"}, {ID: "b", Title: "Color"}}
	dir := t.TempDir()
	model := &fakeModel{}
	create := func() *Service {
		s := New(dir, func() ([]store.Session, error) { return tasks, nil })
		s.load = func(context.Context, string, func(Status)) (embedder, error) { return model, nil }
		return s
	}
	s := create()
	s.Refresh()
	status := waitIndex(t, s)
	if status.Phase != "ready" || status.Total != 2 || status.Version != 1 {
		t.Fatal(status)
	}
	if !reflect.DeepEqual(model.texts, []string{"Login\nFix users\nDone", "Color"}) {
		t.Fatal(model.texts)
	}
	scores, err := s.Search("users", []string{"a", "missing"})
	if err != nil || len(scores) != 1 || scores["a"] != 1 {
		t.Fatal(scores, err)
	}
	_, _ = s.Search("users", []string{"b"})
	if len(model.texts) != 3 {
		t.Fatal("query vector was not reused")
	}
	s.Refresh()
	if waitIndex(t, s).Version != 1 || len(model.texts) != 3 {
		t.Fatal("unchanged tasks re-embedded")
	}
	s.Close()
	if !model.closed {
		t.Fatal("model not closed")
	}
	// A new service restores vectors from disk without inference.
	model = &fakeModel{}
	s = create()
	defer s.Close()
	s.Refresh()
	waitIndex(t, s)
	if len(model.texts) != 0 {
		t.Fatal("disk cache was not reused")
	}
	tasks[0].Summary = "New outcome"
	tasks = tasks[:1]
	s.Refresh()
	status = waitIndex(t, s)
	if status.Total != 1 || len(model.texts) != 1 || model.texts[0] != "Login\nFix users\nNew outcome" {
		t.Fatal(status, model.texts)
	}
	if _, err := os.Stat(s.cachePath("b")); !os.IsNotExist(err) {
		t.Fatal("deleted task remains cached", err)
	}
	scores, err = s.Search("users", []string{"a", "b"})
	if err != nil || len(scores) != 1 {
		t.Fatal(scores, err)
	}
}

func TestConcurrentRefreshRetryAndClose(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	model := &fakeModel{fail: true}
	s := New(t.TempDir(), func() ([]store.Session, error) { return []store.Session{{ID: "a"}}, nil })
	loads := 0
	s.load = func(context.Context, string, func(Status)) (embedder, error) {
		loads++
		close(started)
		<-release
		return model, nil
	}
	s.Refresh()
	<-started
	for i := 0; i < 20; i++ {
		s.Refresh()
	}
	close(release)
	if status := waitIndex(t, s); status.Phase != "error" || status.Error != "inference failed" {
		t.Fatal(status)
	}
	model.fail = false
	s.Refresh()
	if status := waitIndex(t, s); status.Phase != "ready" || loads != 1 {
		t.Fatal(status, loads)
	}
	s.Close()
	s.Refresh()
	if _, err := s.Search("query", []string{"a"}); err == nil {
		t.Fatal("query accepted after close")
	}
}

func TestDownloadCacheAndIncompleteResponse(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("model"))
	}))
	path := filepath.Join(t.TempDir(), "model.onnx")
	if err := download(context.Background(), server.URL, path, func(float64) {}); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := download(context.Background(), server.URL, path, func(float64) {}); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("bad"))
	}))
	defer broken.Close()
	path = filepath.Join(t.TempDir(), "model.onnx")
	if err := download(context.Background(), broken.URL, path, func(float64) {}); err == nil {
		t.Fatal("incomplete download succeeded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("incomplete download cached", err)
	}
}
