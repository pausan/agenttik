package smartsearch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/amikos-tech/pure-onnx/embeddings/minilm"
	"github.com/amikos-tech/pure-onnx/ort"
	tokenizers "github.com/amikos-tech/pure-tokenizers"
)

const revision = "c721113d59a1d91b447450324f51c4b3332c924a"

type embedder interface {
	EmbedDocuments([]string) ([][]float32, error)
	Close() error
}

type nativeModel struct{ *minilm.Embedder }

func (m *nativeModel) Close() error {
	err := m.Embedder.Close()
	return errors.Join(err, ort.DestroyEnvironment())
}

func loadModel(ctx context.Context, dir string, progress func(Status)) (embedder, error) {
	progress(Status{Phase: "runtime"})
	if err := ort.InitializeEnvironmentWithBootstrap(ort.WithBootstrapVersion("1.24.1")); err != nil {
		return nil, fmt.Errorf("prepare ONNX runtime: %w", err)
	}
	loaded := false
	defer func() {
		if !loaded {
			_ = ort.DestroyEnvironment()
		}
	}()
	name := "libtokenizers.so"
	if runtime.GOOS == "darwin" {
		name = "libtokenizers.dylib"
	}
	if runtime.GOOS == "windows" {
		name = "tokenizers.dll"
	}
	library := filepath.Join(dir, "tokenizers-v0.1.4", name)
	if _, err := os.Stat(library); os.IsNotExist(err) {
		if err := tokenizers.DownloadLibraryFromGitHubWithVersion(library, "v0.1.4"); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	modelDir := filepath.Join(dir, revision)
	for _, file := range []string{"tokenizer.json", "onnx/model.onnx"} {
		if err := download(ctx, "https://huggingface.co/hotchpotch/bekko-embedding-v1-a8m/resolve/"+revision+"/"+file,
			filepath.Join(modelDir, file), func(percent float64) { progress(Status{Phase: "download", Percent: percent}) }); err != nil {
			return nil, err
		}
	}
	progress(Status{Phase: "loading"})
	m, err := minilm.NewEmbedder(filepath.Join(modelDir, "onnx/model.onnx"), filepath.Join(modelDir, "tokenizer.json"),
		minilm.WithTokenizerLibraryPath(library), minilm.WithSequenceLength(512),
		minilm.WithInputOutputNames("input_ids", "attention_mask", "", "last_hidden_state"),
		minilm.WithMeanPooling(), minilm.WithL2Normalization(), minilm.WithEmbeddingDimension(384))
	if err != nil {
		return nil, err
	}
	loaded = true
	return &nativeModel{m}, nil
}

// Files become cache hits only after a complete download and atomic rename.
func download(ctx context.Context, url, path string, progress func(float64)) error {
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download model: %s", res.Status)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	progress(0)
	r := &progressReader{Reader: res.Body, total: res.ContentLength, progress: progress}
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	progress(100)
	return nil
}

type progressReader struct {
	io.Reader
	total, read int64
	last        time.Time
	progress    func(float64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += int64(n)
	if r.total > 0 && time.Since(r.last) >= 100*time.Millisecond {
		r.progress(min(99, float64(r.read)/float64(r.total)*100))
		r.last = time.Now()
	}
	return n, err
}
