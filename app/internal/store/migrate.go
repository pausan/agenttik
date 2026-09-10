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
	`
-- The subscription allowance a provider volunteered during the turn, stored as
-- the JSON the API serves. Claude Code only names its buckets on a
-- rate_limit_event line mid-turn, so a reading has to be kept: without this
-- the prompt bar has nothing to show until the next turn happens to report
-- one. Like the two columns above it is never summed — the newest turn that
-- reported a reading is the reading.
ALTER TABLE turns ADD COLUMN rate_limits TEXT NOT NULL DEFAULT '';
	`,
	`
-- A schedule is a prompt plus a clock. It is not a session: no transcript, no
-- provider thread, no turns of its own — what it produces are ordinary
-- sessions. Its own table is what stops every list that reads a session from
-- having to say "unless it is a schedule".
CREATE TABLE schedules (
    id               INTEGER PRIMARY KEY,
    project_id       INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title            TEXT    NOT NULL DEFAULT '',
    prompt           TEXT    NOT NULL,
    provider         TEXT    NOT NULL,
    model            TEXT    NOT NULL,
    effort           TEXT    NOT NULL DEFAULT '',
    permission       TEXT    NOT NULL DEFAULT 'workspace',
    every            TEXT    NOT NULL,            -- interval | day | week | month
    interval_minutes INTEGER NOT NULL DEFAULT 0,  -- the whole X hours Y minutes
    at_minute        INTEGER NOT NULL DEFAULT 0,  -- minutes past local midnight
    anchor_at        INTEGER NOT NULL,            -- fixes the weekday and the day of month
    remaining        INTEGER NOT NULL DEFAULT -1, -- -1 runs forever
    paused           INTEGER NOT NULL DEFAULT 0,
    next_run_at      INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL,
    done_at          INTEGER NOT NULL DEFAULT 0,
    position         INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_schedules_project ON schedules(project_id, position, created_at);
CREATE INDEX idx_schedules_due     ON schedules(next_run_at);

-- One row per fire, spawned or skipped. A skip never becomes a session, so
-- without this table the gap one leaves could not be explained.
CREATE TABLE schedule_runs (
    id          INTEGER PRIMARY KEY,
    schedule_id INTEGER NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    session_id  TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL,   -- running | done | error | interrupted | skipped
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_schedule_runs ON schedule_runs(schedule_id, id DESC);

-- 0 for an ordinary session. A finishing turn reads it to find the schedule
-- that started it, rather than a lookup per turn.
ALTER TABLE sessions ADD COLUMN schedule_id INTEGER NOT NULL DEFAULT 0;
	`,
	`
-- A queued prompt keeps the provider choice it was made with. A session's
-- picker remains the default for its next prompt, while each waiting prompt
-- can independently be inspected or changed before it starts.
ALTER TABLE queued_messages ADD COLUMN provider TEXT NOT NULL DEFAULT '';
ALTER TABLE queued_messages ADD COLUMN model    TEXT NOT NULL DEFAULT '';
ALTER TABLE queued_messages ADD COLUMN effort   TEXT NOT NULL DEFAULT '';
UPDATE queued_messages
SET provider = (SELECT provider FROM sessions WHERE sessions.id = queued_messages.session_id),
    model    = (SELECT model FROM sessions WHERE sessions.id = queued_messages.session_id),
    effort   = (SELECT effort FROM sessions WHERE sessions.id = queued_messages.session_id);
	`,
	`
-- A project can be put away without being deleted. An archived project leaves
-- the sidebar, the Go To list and the scheduler; its tasks, its history and
-- its position are all kept, so restoring it from Settings puts it back where
-- it was. Deleting is still the destructive one. No index: a handful of rows
-- is scanned faster than a second B-tree is read.
ALTER TABLE projects ADD COLUMN archived_at INTEGER NOT NULL DEFAULT 0;
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
