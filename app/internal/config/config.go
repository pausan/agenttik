// Package config resolves where agenttik keeps its data and how it listens.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Addr    string // host:port to listen on
	DataDir string // directory holding agenttik.db
}

// Default returns the configuration used when no flags are given.
func Default() Config {
	return Config{Addr: "127.0.0.1:7717", DataDir: defaultDataDir()}
}

// DBPath is the SQLite file inside the data directory.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "agenttik.db") }

// EnsureDataDir creates the data directory if it does not exist.
func (c Config) EnsureDataDir() error {
	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir %s: %w", c.DataDir, err)
	}
	return nil
}

func defaultDataDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "agenttik")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agenttik"
	}
	return filepath.Join(home, ".local", "share", "agenttik")
}
