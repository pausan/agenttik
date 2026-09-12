package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/pausan/agenttik/app/internal/orchestrator"
)

// OrchestratorConfig reads the live project's options while it exists, or
// the options saved at deletion. Enabled is derived from the project so
// ordinary archive, restore and delete operations cannot leave it stale.
type OrchestratorConfig struct {
	Enabled   bool   `json:"enabled"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Prompt    string `json:"prompt"`
}

func (s *Store) orchestratorDefaults() (OrchestratorConfig, error) {
	path, err := filepath.Abs(filepath.Join(s.dir, "orchestrator"))
	return OrchestratorConfig{Name: "Orchestrator", Path: path, Prompt: orchestrator.DefaultPrompt}, err
}

func (s *Store) GetOrchestratorConfig() (OrchestratorConfig, error) {
	cfg, err := s.orchestratorDefaults()
	if err != nil {
		return cfg, err
	}
	err = s.db.QueryRow(`SELECT id, archived_at = 0, name, path, prompt FROM projects WHERE kind = 'orchestrator'`).
		Scan(&cfg.ProjectID, &cfg.Enabled, &cfg.Name, &cfg.Path, &cfg.Prompt)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRow(`SELECT name, path, prompt FROM orchestrator_config WHERE id = 1`).
			Scan(&cfg.Name, &cfg.Path, &cfg.Prompt)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	}
	return cfg, err
}

// SetOrchestratorEnabled creates at most one project, or archives/restores
// the existing one. The caller prepares the folder before enabling.
func (s *Store) SetOrchestratorEnabled(enabled bool) error {
	if !enabled {
		_, err := s.db.Exec(`UPDATE projects SET archived_at = ? WHERE kind = 'orchestrator' AND archived_at = 0`, nowMillis())
		return err
	}
	defaults, err := s.orchestratorDefaults()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO projects (name, path, prompt, created_at, kind)
		SELECT COALESCE((SELECT name FROM orchestrator_config WHERE id = 1), ?),
		       COALESCE((SELECT path FROM orchestrator_config WHERE id = 1), ?),
		       COALESCE((SELECT prompt FROM orchestrator_config WHERE id = 1), ?), ?, 'orchestrator'
		WHERE NOT EXISTS (SELECT 1 FROM projects WHERE kind = 'orchestrator')`,
		defaults.Name, defaults.Path, defaults.Prompt, nowMillis())
	if pathTaken(err) {
		return ErrPathInUse
	}
	if err != nil {
		return fmt.Errorf("create orchestrator: %w", err)
	}
	if _, err := tx.Exec(`UPDATE projects SET archived_at = 0 WHERE kind = 'orchestrator'`); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetOrchestratorPrompt also works after deletion, without enabling the
// project. An intentionally empty prompt is preserved until explicitly reset.
func (s *Store) ResetOrchestratorPrompt() error {
	defaults, err := s.orchestratorDefaults()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO orchestrator_config (id, name, path, prompt) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET prompt = excluded.prompt`, defaults.Name, defaults.Path, defaults.Prompt)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE projects SET prompt = ? WHERE kind = 'orchestrator'`, defaults.Prompt); err != nil {
		return err
	}
	return tx.Commit()
}

// HasProjectWork is shared by project archiving and the orchestrator switch.
// A queued retry counts too: hiding it would hide the only way to stop it.
func (s *Store) HasProjectWork(id int64) (bool, error) {
	var busy bool
	err := s.db.QueryRow(`SELECT
		EXISTS (SELECT 1 FROM sessions s WHERE s.project_id = ? AND
		  (s.status = 'running' OR EXISTS (SELECT 1 FROM queued_messages q WHERE q.session_id = s.id)))
		OR EXISTS (SELECT 1 FROM schedule_runs r JOIN schedules j ON j.id = r.schedule_id
		  WHERE j.project_id = ? AND r.status = 'running')`, id, id).Scan(&busy)
	return busy, err
}
