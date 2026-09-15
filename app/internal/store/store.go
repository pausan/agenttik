// Package store is the SQLite persistence layer. It holds queries only; the
// turn lifecycle and provider logic live elsewhere.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
	// dir is the data directory the database sits in. Kept because it is also
	// where app-managed directories belong — a subscription's CLI config dir,
	// for one. See 050-subscription-accounts.md.
	dir string
}

// Open opens (creating if needed) the database at path and migrates it.
func Open(path string) (*Store, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// modernc/sqlite is safe for concurrent use but a single writer avoids
	// SQLITE_BUSY churn; reads are fast enough that this is not a bottleneck.
	db.SetMaxOpenConns(1)
	if err := migrate(db, absolutePath); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, dir: filepath.Dir(path), path: absolutePath}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Dir is the data directory holding the database.
func (s *Store) Dir() string { return s.dir }

// Path is the absolute path of the database file.
func (s *Store) Path() string { return s.path }

// DB exposes the handle for tests.
func (s *Store) DB() *sql.DB { return s.db }

func nowMillis() int64 { return time.Now().UnixMilli() }
