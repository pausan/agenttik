package store

import (
	"path/filepath"
	"testing"
)

func TestDesktopConfigPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := DesktopConfig{true, "Alt+Shift+F12"}
	if err := s.SetDesktopConfig(want); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetDesktopConfig()
	if err != nil || got != want {
		t.Fatalf("reopened: %+v, %v", got, err)
	}
}

func TestDesktopShortcut(t *testing.T) {
	for _, chord := range []string{"Ctrl+Shift+A", "Alt+0", "Shift+F1", "Ctrl+Alt+Shift+F12"} {
		if err := ValidateDesktopShortcut(chord); err != nil {
			t.Errorf("%q: %v", chord, err)
		}
	}
	for _, chord := range []string{"", "A", "Ctrl+", "Ctrl+a", "Ctrl+Ctrl+A", "Meta+A", "Ctrl+F0", "Ctrl+F13", "Ctrl+Shift", "Ctrl+A+B", "Ctrl+Q"} {
		if ValidateDesktopShortcut(chord) == nil {
			t.Errorf("accepted %q", chord)
		}
	}
}
