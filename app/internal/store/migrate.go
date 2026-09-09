package store

import (
	"database/sql"
	"fmt"
)

// migrations run in order. Append only; never edit a released step.
var migrations = []string{
	`
CREATE TABLE projects (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL,
    path       TEXT    NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);

CREATE TABLE sessions (
    id                  TEXT    PRIMARY KEY,
    project_id          INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title               TEXT    NOT NULL DEFAULT '',
    provider            TEXT    NOT NULL,
    provider_session_id TEXT    NOT NULL DEFAULT '',
    model               TEXT    NOT NULL,
    effort              TEXT    NOT NULL DEFAULT '',
    permission          TEXT    NOT NULL DEFAULT 'workspace',
    source              TEXT    NOT NULL DEFAULT 'agenttik',
    status              TEXT    NOT NULL DEFAULT 'idle',
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    last_active_at      INTEGER NOT NULL
);
CREATE INDEX idx_sessions_project ON sessions(project_id, last_active_at DESC);
CREATE INDEX idx_sessions_active  ON sessions(last_active_at DESC);

CREATE TABLE turns (
    id                 INTEGER PRIMARY KEY,
    session_id         TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    model              TEXT    NOT NULL,
    effort             TEXT    NOT NULL DEFAULT '',
    started_at         INTEGER NOT NULL,
    ended_at           INTEGER,
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd           REAL    NOT NULL DEFAULT 0,
    status             TEXT    NOT NULL DEFAULT 'running',
    error              TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_turns_session ON turns(session_id, id);

CREATE TABLE messages (
    id         INTEGER PRIMARY KEY,
    session_id TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    turn_id    INTEGER,
    role       TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_messages_session ON messages(session_id, id);

CREATE TABLE starred_models (
    provider   TEXT    NOT NULL,
    model      TEXT    NOT NULL,
    effort     TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (provider, model, effort)
);
`,
	`
-- A session can be ticked off. Done sessions leave the project views and stay
-- in the Sessions list, so position only ever orders the ones still open.
ALTER TABLE sessions ADD COLUMN done_at  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_sessions_order ON sessions(project_id, position, last_active_at DESC);

-- The size of the last prompt the provider actually sent, which is what the
-- context gauge compares against the model's window. Unlike the token columns
-- next to it this is not a sum: a turn with twenty tool calls sends twenty
-- prompts, and only the last one describes the context in use now.
ALTER TABLE turns ADD COLUMN context_tokens INTEGER NOT NULL DEFAULT 0;
	`,
	`
-- Projects follow the order set in the sidebar. As with sessions, 0 leaves a
-- newly created project ahead of a manually ordered list.
ALTER TABLE projects ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_projects_order ON projects(position, name);
	`,
	`
-- Queued prompts wait for the project runner. They are separate from turns
-- because a turn only exists once a provider is actually started.
CREATE TABLE queued_messages (
    id         INTEGER PRIMARY KEY,
    session_id TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    prompt     TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_queued_messages_session ON queued_messages(session_id, id);
	`,
	`
-- The window the provider says it ran the model in. The static per-model
-- figure is only a guess: the same alias runs in a 200k or a 1M variant, and
-- --autocompact moves the ceiling again. Like context_tokens next to it this
-- is not a sum; the newest turn that reported one describes the gauge now.
ALTER TABLE turns ADD COLUMN context_window INTEGER NOT NULL DEFAULT 0;
	`,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		// PRAGMA does not accept placeholders.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: set version: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d: commit: %w", i+1, err)
		}
	}
	return nil
}
