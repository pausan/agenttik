package store

import (
	"fmt"
	"strings"
)

type DesktopConfig struct {
	CloseToTray    bool   `json:"close_to_tray"`
	ToggleShortcut string `json:"toggle_shortcut"`
}

// ValidateDesktopShortcut keeps the portable shortcut vocabulary explicit.
func ValidateDesktopShortcut(chord string) error {
	if chord == "Ctrl+Q" || chord == "Cmd+Q" {
		return fmt.Errorf("%s is reserved for quitting", chord)
	}
	parts := strings.Split(chord, "+")
	if len(parts) < 2 {
		return fmt.Errorf("shortcut needs Ctrl, Cmd, Alt or Shift and a letter, digit or F1–F12")
	}
	seen := map[string]bool{}
	for _, mod := range parts[:len(parts)-1] {
		if (mod != "Ctrl" && mod != "Cmd" && mod != "Alt" && mod != "Shift") || seen[mod] {
			return fmt.Errorf("invalid or repeated shortcut modifier: %s", mod)
		}
		seen[mod] = true
	}
	key := parts[len(parts)-1]
	if len(key) == 1 && ((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= '0' && key[0] <= '9')) {
		return nil
	}
	for n := 1; n <= 12; n++ {
		if key == fmt.Sprintf("F%d", n) {
			return nil
		}
	}
	return fmt.Errorf("shortcut key must be A–Z, 0–9 or F1–F12")
}

func (s *Store) GetDesktopConfig() (DesktopConfig, error) {
	var c DesktopConfig
	err := s.db.QueryRow(`SELECT close_to_tray, toggle_shortcut FROM desktop_config WHERE id = 1`).Scan(&c.CloseToTray, &c.ToggleShortcut)
	return c, err
}

func (s *Store) SetDesktopConfig(c DesktopConfig) error {
	if err := ValidateDesktopShortcut(c.ToggleShortcut); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE desktop_config SET close_to_tray = ?, toggle_shortcut = ? WHERE id = 1`, c.CloseToTray, c.ToggleShortcut)
	return err
}
