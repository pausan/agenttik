package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestServerName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.db")
	s, err := Open(path)
	must(t, err)
	original, err := s.ServerName()
	must(t, err)
	if !strings.HasPrefix(original, "agenttik-") {
		t.Fatalf("default name: %q", original)
	}
	other := testStore(t)
	otherName, err := other.ServerName()
	must(t, err)
	if original == otherName {
		t.Fatal("instances received the same name")
	}
	must(t, s.Close())
	s, err = Open(path)
	must(t, err)
	got, err := s.ServerName()
	must(t, err)
	if got != original {
		t.Fatal("generated name changed after reopening")
	}
	for _, name := range []string{"", "  ", "ab", "  ab  ", "猫犬"} {
		if s.SetServerName(name) == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	must(t, s.SetServerName("  猫犬鳥  "))
	must(t, s.Close())
	s, err = Open(path)
	must(t, err)
	defer s.Close()
	got, err = s.ServerName()
	must(t, err)
	if got != "猫犬鳥" {
		t.Fatalf("saved name: %q", got)
	}
}

func TestMachineIDIsStableAndUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.db")
	s, err := Open(path)
	must(t, err)
	id, err := s.MachineID()
	must(t, err)
	if len(id) != 32 {
		t.Fatalf("machine id: %q", id)
	}
	other, err := testStore(t).MachineID()
	must(t, err)
	if other == id {
		t.Fatal("instances received the same machine id")
	}
	must(t, s.SetServerName("Renamed"))
	must(t, s.Close())
	s, err = Open(path)
	must(t, err)
	defer s.Close()
	got, err := s.MachineID()
	must(t, err)
	if got != id {
		t.Fatal("machine id changed after reopening")
	}
}
