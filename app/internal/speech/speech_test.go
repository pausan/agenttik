package speech

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestDownloadVerifiedAtomicAndCached(t *testing.T) {
	data := []byte("model weights")
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write(data) }))
	defer server.Close()
	s := New(t.TempDir())
	defer s.Close()
	path := filepath.Join(s.dir, "model")
	if err := s.download(context.Background(), server.URL, path, int64(len(data)), strings.Repeat("0", 64), 0600); err == nil {
		t.Fatal("accepted wrong checksum")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("published invalid model")
	}
	files, _ := os.ReadDir(s.dir)
	if len(files) != 0 {
		t.Fatal("left partial downloads")
	}
	for i := 0; i < 2; i++ {
		if err := s.download(context.Background(), server.URL, path, int64(len(data)), hash, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("requests=%d; cached download was fetched again", calls)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Fatal("wrong bytes")
	}
}

func cachedWhistle(t *testing.T) *Service {
	t.Helper()
	a, ok := whistleRuntime[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		t.Skip("no Whistle runtime for this platform")
	}
	s := New(t.TempDir())
	t.Cleanup(s.Close)
	m, _ := Lookup("whistle")
	for name, size := range map[string]int64{m.file: m.Bytes, "needle-3.3.3" + exeSuffix(): a.size} {
		f, err := os.Create(filepath.Join(s.dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	return s
}

func TestTranscribeBoundedWAVAndCleanup(t *testing.T) {
	s := cachedWhistle(t)
	pcm := []byte{0, 0, 255, 127, 0, 128}
	var recording string
	s.run = func(ctx context.Context, helper string, args ...string) ([]byte, error) {
		if !strings.HasSuffix(helper, "needle-3.3.3"+exeSuffix()) || len(args) != 4 || args[0] != "--model" || args[2] != "--audio" {
			t.Fatalf("bad command %s %v", helper, args)
		}
		recording = args[3]
		wav, err := os.ReadFile(recording)
		if err != nil {
			t.Fatal(err)
		}
		if string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " || binary.LittleEndian.Uint32(wav[24:]) != 16000 || binary.LittleEndian.Uint16(wav[22:]) != 1 || !bytes.Equal(wav[44:], pcm) {
			t.Fatal("invalid native audio format")
		}
		return []byte(`{"text":" Hola mundo. ","language":"es"}`), nil
	}
	text, err := s.Transcribe(context.Background(), "whistle", pcm)
	if err != nil || text != "Hola mundo." {
		t.Fatalf("%q %v", text, err)
	}
	if _, err := os.Stat(filepath.Dir(recording)); !os.IsNotExist(err) {
		t.Fatal("recording retained")
	}
	for _, invalid := range [][]byte{nil, {1}, make([]byte, MaxSamples*2+2)} {
		if _, err := s.Transcribe(context.Background(), "whistle", invalid); err == nil {
			t.Fatal("accepted invalid recording")
		}
	}
	if err := s.Prepare(context.Background(), "../../model"); err == nil {
		t.Fatal("accepted arbitrary model")
	}
}

func TestSerializationAndCancellation(t *testing.T) {
	s := cachedWhistle(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s.run = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		once.Do(func() { close(started) })
		select {
		case <-release:
			return []byte(`{"text":"ok"}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	done := make(chan struct{})
	go func() { defer close(done); s.Transcribe(context.Background(), "whistle", []byte{0, 0}) }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Prepare(ctx, "whistle"); err == nil {
		t.Fatal("queued prepare ignored cancellation")
	}
	s.Close()
	<-done
	close(release)
}
