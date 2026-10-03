package agent

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// LoadRemembered reads into v what SaveRemembered last wrote to path. A
// missing or unreadable file leaves v as it was.
func LoadRemembered(path string, v any) {
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, v)
	}
}

// SaveRemembered replaces path with v as JSON. The file is renamed into
// place, so a crash mid-write leaves the previous answer rather than half of
// one. A failure is only logged: the next launch asks again.
func SaveRemembered(path string, v any) {
	if err := saveRemembered(path, v); err != nil {
		log.Printf("remember %s: %v", filepath.Base(path), err)
	}
}

func saveRemembered(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agenttik-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
