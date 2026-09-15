package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func rawDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func execSQL(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func scalar(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	var value string
	if err := db.QueryRow(query).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func frozenMigrations(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/migrations.json")
	if err != nil {
		t.Fatal(err)
	}
	var frozen []string
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatal(err)
	}
	if len(migrations) < len(frozen) || !reflect.DeepEqual(migrations[:len(frozen)], frozen) {
		t.Fatal("historical migrations changed: append new steps; never edit or remove existing steps")
	}
	return append(frozen, migrations[len(frozen):]...)
}

func TestUpgradeEverySchemaVersion(t *testing.T) {
	history := frozenMigrations(t)
	fresh := testStore(t)
	schemaQuery := "SELECT group_concat(sql, ';') FROM (SELECT sql FROM sqlite_schema WHERE sql IS NOT NULL ORDER BY name)"
	wantSchema := scalar(t, fresh.db, schemaQuery)
	for version := 0; version <= len(history); version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agenttik.db")
			old := rawDatabase(t, path)
			if err := applyMigrations(old, 0, history[:version]); err != nil {
				t.Fatal(err)
			}
			if version > 0 {
				execSQL(t, old, `INSERT INTO projects(id,name,path,created_at) VALUES(1,'project','/project',123);
INSERT INTO sessions(id,project_id,provider,model,effort,created_at,updated_at,last_active_at) VALUES('task',1,'codex','model','high',123,124,125);
INSERT INTO turns(id,session_id,model,started_at,input_tokens) VALUES(1,'task','model',123,42);
INSERT INTO messages(session_id,turn_id,role,content,created_at) VALUES('task',1,'user','keep me',123);
INSERT INTO starred_models(provider,model,effort,created_at) VALUES('codex','model','high',123);`)
				if version >= 4 {
					execSQL(t, old, `INSERT INTO queued_messages(session_id,prompt,created_at) VALUES('task','queued',123)`)
				}
			}
			// Keep the old connection open so the backup must include WAL contents.
			upgraded, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			if got := scalar(t, upgraded.db, "PRAGMA user_version"); got != fmt.Sprint(len(migrations)) {
				t.Fatal(got)
			}
			if got := scalar(t, upgraded.db, schemaQuery); got != wantSchema {
				t.Fatal("upgraded schema differs from fresh schema")
			}
			if got := scalar(t, upgraded.db, "PRAGMA integrity_check"); got != "ok" {
				t.Fatal(got)
			}
			rows, err := upgraded.db.Query("PRAGMA foreign_key_check")
			if err != nil {
				t.Fatal(err)
			}
			if rows.Next() {
				t.Fatal("broken foreign key")
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			rows.Close()
			if version > 0 {
				for query, want := range map[string]string{
					"SELECT name FROM projects WHERE id=1":        "project",
					"SELECT effort FROM sessions WHERE id='task'": "high",
					"SELECT input_tokens FROM turns WHERE id=1":   "42",
					"SELECT content FROM messages":                "keep me",
					"SELECT account_id FROM starred_models":       "0",
					"SELECT created_at FROM starred_models":       "123",
				} {
					if got := scalar(t, upgraded.db, query); got != want {
						t.Fatalf("%s: got %s, want %s", query, got, want)
					}
				}
				if version >= 4 && version < 8 {
					if got := scalar(t, upgraded.db, "SELECT provider || ':' || model || ':' || effort FROM queued_messages"); got != "codex:model:high" {
						t.Fatal(got)
					}
				}
			}
			backups, err := filepath.Glob(path + ".before-*.db")
			if err != nil {
				t.Fatal(err)
			}
			wantBackups := 0
			if version > 0 && version < len(migrations) {
				wantBackups = 1
			}
			if len(backups) != wantBackups {
				t.Fatalf("backups: %v", backups)
			}
			if wantBackups == 1 {
				backup := rawDatabase(t, backups[0])
				if got := scalar(t, backup, "PRAGMA user_version"); got != fmt.Sprint(version) {
					t.Fatal(got)
				}
				if got := scalar(t, backup, "SELECT content FROM messages"); got != "keep me" {
					t.Fatal("backup lost WAL data")
				}
				if got := scalar(t, backup, "PRAGMA integrity_check"); got != "ok" {
					t.Fatal(got)
				}
			}
			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			reopened.Close()
			after, _ := filepath.Glob(path + ".before-*.db")
			if len(after) != wantBackups {
				t.Fatal("reopening created a backup")
			}
		})
	}
}

func TestMigrationRejectsUnsupportedVersion(t *testing.T) {
	for _, version := range []int{-1, len(migrations) + 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "db")
			db := rawDatabase(t, path)
			execSQL(t, db, fmt.Sprintf("PRAGMA user_version=%d", version))
			s, err := Open(path)
			if s != nil {
				s.Close()
				t.Fatal("opened unsupported schema")
			}
			if err == nil || !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("error: %v", err)
			}
			if got := scalar(t, db, "PRAGMA user_version"); got != fmt.Sprint(version) {
				t.Fatal("version changed")
			}
			backups, _ := filepath.Glob(path + ".before-*")
			if len(backups) != 0 {
				t.Fatal(backups)
			}
		})
	}
}

func TestMigrationFailureRollsBackAndRetries(t *testing.T) {
	db := rawDatabase(t, filepath.Join(t.TempDir(), "db"))
	steps := []string{"CREATE TABLE kept (value TEXT); INSERT INTO kept VALUES ('original');", "UPDATE kept SET value='changed'; CREATE TABLE partial(id INTEGER); INSERT INTO missing VALUES(1);"}
	if err := applyMigrations(db, 0, steps); err == nil {
		t.Fatal("expected migration failure")
	}
	if got := scalar(t, db, "PRAGMA user_version"); got != "1" {
		t.Fatal(got)
	}
	if got := scalar(t, db, "SELECT value FROM kept"); got != "original" {
		t.Fatal(got)
	}
	if got := scalar(t, db, "SELECT count(*) FROM sqlite_schema WHERE name='partial'"); got != "0" {
		t.Fatal("DDL was not rolled back")
	}
	execSQL(t, db, "CREATE TABLE missing(id INTEGER)")
	if err := applyMigrations(db, 1, steps); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, db, "PRAGMA user_version"); got != "2" {
		t.Fatal(got)
	}
	if got := scalar(t, db, "SELECT value FROM kept"); got != "changed" {
		t.Fatal(got)
	}
}

func TestBackupFailureStopsMigration(t *testing.T) {
	db := rawDatabase(t, filepath.Join(t.TempDir(), "db"))
	if err := applyMigrations(db, 0, migrations[:1]); err != nil {
		t.Fatal(err)
	}
	err := migrate(db, filepath.Join(t.TempDir(), "missing", "db"))
	if err == nil || !strings.Contains(err.Error(), "back up database") {
		t.Fatalf("error: %v", err)
	}
	if got := scalar(t, db, "PRAGMA user_version"); got != "1" {
		t.Fatal(got)
	}
	if got := scalar(t, db, "SELECT count(*) FROM pragma_table_info('sessions') WHERE name='done_at'"); got != "0" {
		t.Fatal("schema changed")
	}
}

func TestFailedUpgradeKeepsBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	db := rawDatabase(t, path)
	if err := applyMigrations(db, 0, migrations[:1]); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, "CREATE INDEX idx_sessions_order ON sessions(title)")
	err := migrate(db, path)
	if err == nil || !strings.Contains(err.Error(), "pre-upgrade backup:") {
		t.Fatalf("error: %v", err)
	}
	if got := scalar(t, db, "PRAGMA user_version"); got != "1" {
		t.Fatal(got)
	}
	backups, _ := filepath.Glob(path + ".before-*.db")
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	execSQL(t, db, "DROP INDEX idx_sessions_order")
	if err := migrate(db, path); err != nil {
		t.Fatal(err)
	}
	after, _ := filepath.Glob(path + ".before-*.db")
	if len(after) != 2 {
		t.Fatal("retry overwrote recovery backup")
	}
}

func TestIncompleteBackupIsRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	db := rawDatabase(t, path)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := backupBeforeMigration(db, path, 1); err == nil {
		t.Fatal("expected snapshot failure")
	}
	files, err := filepath.Glob(path + ".before-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("incomplete backup left behind: %v", files)
	}
}
