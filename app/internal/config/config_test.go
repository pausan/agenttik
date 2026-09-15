package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemporaryDataIsolationAndCleanup(t *testing.T) {
	normal := t.TempDir()
	sentinel := filepath.Join(normal, "agenttik.db")
	if err := os.WriteFile(sentinel, []byte("normal"), 0600); err != nil {
		t.Fatal(err)
	}
	a, b := Config{DataDir: normal}, Config{DataDir: normal}
	cleanupA, err := a.UseTemporaryData()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupA()
	cleanupB, err := b.UseTemporaryData()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupB()
	if a.DataDir == b.DataDir || a.DataDir == normal {
		t.Fatal("shared directory")
	}
	if err := os.WriteFile(a.DBPath(), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	cleanupA()
	if _, err := os.Stat(a.DataDir); !os.IsNotExist(err) {
		t.Fatalf("private data remains: %v", err)
	}
	if _, err := os.Stat(b.DataDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "normal" {
		t.Fatalf("normal data changed: %q %v", data, err)
	}
}
