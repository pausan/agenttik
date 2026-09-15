package update

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleRejectsUnsafeEntries(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "agenttik.app/../../outside", "agenttik.app/Contents/link"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "bundle.zip")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(f)
			header := &zip.FileHeader{Name: name}
			if strings.HasSuffix(name, "/link") {
				header.SetMode(os.ModeSymlink | 0777)
			}
			w, err := zw.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			w.Write([]byte("outside"))
			zw.Close()
			f.Close()
			_, err = unpackBundle(context.Background(), archive, dir)
			if err == nil || (!strings.Contains(err.Error(), "unexpected path") && !strings.Contains(err.Error(), "symlinks")) {
				t.Fatalf("unsafe archive reached extraction: %v", err)
			}
		})
	}
}
