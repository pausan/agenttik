package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShareAccountsMigratesCollidingIDsAndManagedHomes(t *testing.T) {
	root, err := Open(filepath.Join(t.TempDir(), "root.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	profileDir := filepath.Join(root.Dir(), "profiles", "work")
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		t.Fatal(err)
	}
	local, err := Open(filepath.Join(profileDir, "profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	original := &Account{Provider: "fake", Alias: "Work", Home: t.TempDir()}
	if err := root.CreateAccount(original); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"Work", "Other"} {
		home := filepath.Join(profileDir, "accounts", "fake", alias)
		if err := os.MkdirAll(home, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "login"), []byte(alias), 0600); err != nil {
			t.Fatal(err)
		}
		a := &Account{Provider: "fake", Alias: alias, Home: home}
		if err := local.CreateAccount(a); err != nil {
			t.Fatal(err)
		}
		if err := local.AddStar("fake", a.ID, alias, ""); err != nil {
			t.Fatal(err)
		}
		if err := local.SetModelChoiceHidden("fake", a.ID, alias, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := local.ShareAccounts(root); err != nil {
		t.Fatal(err)
	}
	accounts, err := local.ListAccounts()
	if err != nil || len(accounts) != 3 {
		t.Fatalf("accounts: %+v %v", accounts, err)
	}
	stars, err := local.ListStars()
	if err != nil || len(stars) != 2 {
		t.Fatalf("stars: %+v %v", stars, err)
	}
	for _, star := range stars {
		a, err := local.GetAccount(star.AccountID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(a.Home, "login"))
		if err != nil || string(data) != star.Model {
			t.Fatalf("wrong login for %+v: %s %v", star, data, err)
		}
		if a.ID == original.ID {
			t.Fatal("old reference resolved to root account")
		}
	}
	local.accounts = nil // Reopening the store must not import again.
	if err := local.ShareAccounts(root); err != nil {
		t.Fatal(err)
	}
	accounts, _ = root.ListAccounts()
	if len(accounts) != 3 {
		t.Fatal("import repeated", accounts)
	}
	if stars, _ := root.ListStars(); len(stars) != 0 {
		t.Fatal("favourites became shared")
	}
}

func TestShareAccountsKeepsDeletedAccountsUnresolved(t *testing.T) {
	root := testStore(t)
	local := testStore(t)
	a := &Account{Provider: "fake", Alias: "Root", Home: t.TempDir()}
	if err := root.CreateAccount(a); err != nil {
		t.Fatal(err)
	}
	// A legacy task preference can still refer to a removed local subscription
	// whose ID now happens to belong to a different account in the root store.
	if err := local.AddStar("fake", a.ID, "model", ""); err != nil {
		t.Fatal(err)
	}
	if err := local.ShareAccounts(root); err != nil {
		t.Fatal(err)
	}
	stars, err := local.ListStars()
	if err != nil || len(stars) != 1 {
		t.Fatalf("stars: %+v %v", stars, err)
	}
	if _, err := local.GetAccount(stars[0].AccountID); err != ErrNotFound {
		t.Fatalf("deleted account resolved: %v", err)
	}
	local.accounts = nil
	if err := local.ShareAccounts(root); err != nil {
		t.Fatal(err)
	}
	again, _ := local.ListStars()
	if again[0].AccountID != stars[0].AccountID {
		t.Fatal("migration repeated")
	}
}
