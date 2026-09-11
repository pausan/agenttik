package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pausan/agenttik/app/internal/config"
	"github.com/pausan/agenttik/app/internal/single"
	"github.com/pausan/agenttik/app/internal/store"
)

// addProject is --init: put a folder in the project list from the terminal,
// without hunting for the Add Project dialog. It works whether or not an
// instance is up, which is the point — the folder you are standing in is the
// one you want, and whether you happen to have the app open is beside it.
//
// The running instance owns the database, so when there is one the folder
// goes through its API and every open window redraws. When there is none this
// process takes the lock and writes the row itself, then exits: --init adds a
// project, it does not start the app.
func addProject(cfg config.Config, dir string) error {
	path, err := projectDir(dir)
	if err != nil {
		return err
	}

	lock, err := single.Acquire(cfg.LockPath())
	if errors.Is(err, single.ErrHeld) {
		return addProjectRemote(single.Addr(cfg.LockPath()), path)
	}
	if err != nil {
		return err
	}
	defer lock.Release()

	db, err := store.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.CreateProject(filepath.Base(path), path); err != nil {
		if errors.Is(err, store.ErrPathInUse) {
			return alreadyAdded(path)
		}
		return err
	}
	return added(path)
}

// projectDir resolves what the user typed, or the working directory when they
// typed nothing. It resolves here rather than at the far end because the
// instance that receives it was started somewhere else entirely, so a relative
// path means nothing by the time it arrives.
func projectDir(dir string) (string, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = wd
	}
	path, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("invalid folder %s: %w", dir, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return path, nil
}

// addProjectRemote hands the folder to the instance already running. addr is
// empty when that one holds the lock but has not started listening yet, which
// is the one case with nowhere to send it and no database to fall back on.
func addProjectRemote(addr, path string) error {
	if addr == "" {
		return errors.New("agenttik is starting up; try again in a moment")
	}
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return err
	}
	client := http.Client{Timeout: raiseTimeout}
	res, err := client.Post("http://"+addr+"/api/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("reach agenttik on %s: %w", addr, err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusCreated:
		return added(path)
	case http.StatusConflict: // the path column is unique; nothing else conflicts here
		return alreadyAdded(path)
	}
	var answer struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&answer); err == nil && answer.Error != "" {
		return errors.New(answer.Error)
	}
	return fmt.Errorf("agenttik answered %s", res.Status)
}

// added and alreadyAdded report the two endings that are not failures. Running
// --init twice is not an error: the folder is a project either way.
func added(path string) error {
	fmt.Printf("added %s to agenttik\n", path)
	return nil
}

func alreadyAdded(path string) error {
	fmt.Printf("%s is already a project\n", path)
	return nil
}
