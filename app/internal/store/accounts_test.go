package store

import "testing"

func TestAccountDefaultIsExclusivePerProvider(t *testing.T) {
	s := testStore(t)
	work := &Account{Provider: "claude", Alias: "Work", Home: "/tmp/work"}
	personal := &Account{Provider: "claude", Alias: "Personal", Home: "/tmp/personal"}
	other := &Account{Provider: "codex", Alias: "Work", Home: "/tmp/codex-work", IsDefault: true}
	for _, a := range []*Account{work, personal, other} {
		if err := s.CreateAccount(a); err != nil {
			t.Fatalf("create %s: %v", a.Alias, err)
		}
	}

	// Nothing flagged yet for claude, so a new task lands on the CLI's login.
	if id, err := s.DefaultAccount("claude"); err != nil || id != SystemAccount {
		t.Fatalf("default before any choice = %d, %v; want the system account", id, err)
	}
	if err := s.SetDefaultAccount("claude", work.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if err := s.SetDefaultAccount("claude", personal.ID); err != nil {
		t.Fatalf("set default again: %v", err)
	}
	if id, _ := s.DefaultAccount("claude"); id != personal.ID {
		t.Errorf("default = %d, want the one set last (%d)", id, personal.ID)
	}
	// One default per provider: choosing the second cleared the first.
	if a, _ := s.GetAccount(work.ID); a.IsDefault {
		t.Errorf("%s is still flagged default", a.Alias)
	}
	// And another provider's choice was not disturbed by either.
	if id, _ := s.DefaultAccount("codex"); id != other.ID {
		t.Errorf("codex default = %d, want %d", id, other.ID)
	}

	// The system account is how the choice is undone, and it has no row to
	// carry the flag.
	if err := s.SetDefaultAccount("claude", SystemAccount); err != nil {
		t.Fatalf("back to system: %v", err)
	}
	if id, _ := s.DefaultAccount("claude"); id != SystemAccount {
		t.Errorf("default = %d, want the system account", id)
	}
}

func TestAccountAliasIsUniquePerProvider(t *testing.T) {
	s := testStore(t)
	if err := s.CreateAccount(&Account{Provider: "claude", Alias: "Work", Home: "/tmp/a"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// The alias is how two subscriptions are told apart in every picker.
	if err := s.CreateAccount(&Account{Provider: "claude", Alias: "Work", Home: "/tmp/b"}); err == nil {
		t.Error("a second Work on the same provider was accepted")
	}
	// Another provider may reuse it: they are never offered side by side
	// without their provider's name.
	if err := s.CreateAccount(&Account{Provider: "codex", Alias: "Work", Home: "/tmp/c"}); err != nil {
		t.Errorf("Work on another provider: %v", err)
	}
}

func TestAccountUsageCountsWhatWouldBreak(t *testing.T) {
	s := testStore(t)
	project, err := s.CreateProject("p", "/tmp/p")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	account := &Account{Provider: "claude", Alias: "Work", Home: "/tmp/work"}
	if err := s.CreateAccount(account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := s.CreateSession(&Session{ID: "a", ProjectID: project.ID, Provider: "claude",
		AccountID: account.ID, Model: "opus"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	// A task on the machine's own login is not this subscription's.
	if err := s.CreateSession(&Session{ID: "b", ProjectID: project.ID, Provider: "claude",
		Model: "opus"}); err != nil {
		t.Fatalf("create system session: %v", err)
	}
	if err := s.CreateSchedule(&Schedule{ProjectID: project.ID, Prompt: "hi", Provider: "claude",
		AccountID: account.ID, Model: "opus", Every: "day"}); err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	sessions, schedules, err := s.AccountUsage(account.ID)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if sessions != 1 || schedules != 1 {
		t.Errorf("usage = %d tasks, %d jobs; want 1 and 1", sessions, schedules)
	}
	if sessions, schedules, _ := s.AccountUsage(SystemAccount); sessions != 0 || schedules != 0 {
		t.Errorf("the system account counts nothing of its own, got %d and %d", sessions, schedules)
	}
}

func TestSessionKeepsItsAccountAcrossAModelChange(t *testing.T) {
	s := testStore(t)
	project, err := s.CreateProject("p", "/tmp/p")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	account := &Account{Provider: "claude", Alias: "Work", Home: "/tmp/work"}
	if err := s.CreateAccount(account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	sess := &Session{ID: "a", ProjectID: project.ID, Provider: "claude",
		AccountID: account.ID, Model: "opus", ProviderSessionID: "thread-1"}
	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A model change inside one subscription keeps the conversation.
	if err := s.SetSessionModel("a", "claude", account.ID, "haiku", "low", false); err != nil {
		t.Fatalf("set model: %v", err)
	}
	got, err := s.GetSession("a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AccountID != account.ID || got.ProviderSessionID != "thread-1" {
		t.Errorf("after a model change: account %d, thread %q; want %d and thread-1",
			got.AccountID, got.ProviderSessionID, account.ID)
	}

	// Swapping subscription starts a new one: the thread belongs to the
	// account that opened it.
	if err := s.SetSessionModel("a", "claude", SystemAccount, "haiku", "low", true); err != nil {
		t.Fatalf("swap subscription: %v", err)
	}
	got, _ = s.GetSession("a")
	if got.AccountID != SystemAccount || got.ProviderSessionID != "" {
		t.Errorf("after a swap: account %d, thread %q; want the system account and no thread",
			got.AccountID, got.ProviderSessionID)
	}
}

func TestRateLimitsAreRememberedPerAccount(t *testing.T) {
	s := testStore(t)
	project, err := s.CreateProject("p", "/tmp/p")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	account := &Account{Provider: "claude", Alias: "Work", Home: "/tmp/work"}
	if err := s.CreateAccount(account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	for _, sess := range []*Session{
		{ID: "sys", ProjectID: project.ID, Provider: "claude", Model: "opus"},
		{ID: "work", ProjectID: project.ID, Provider: "claude", AccountID: account.ID, Model: "opus"},
	} {
		if err := s.CreateSession(sess); err != nil {
			t.Fatalf("create session %s: %v", sess.ID, err)
		}
		turn, err := s.StartTurn(sess.ID, "opus", "")
		if err != nil {
			t.Fatalf("start turn: %v", err)
		}
		turn.Status = "done"
		turn.RateLimits = `[{"limit_id":"` + sess.ID + `"}]`
		if err := s.FinishTurn(turn); err != nil {
			t.Fatalf("finish turn: %v", err)
		}
	}

	// Each subscription reads back its own allowance, never the other's,
	// even though the newest turn overall belongs to one of them.
	for _, tc := range []struct {
		account int64
		want    string
	}{
		{SystemAccount, `[{"limit_id":"sys"}]`},
		{account.ID, `[{"limit_id":"work"}]`},
	} {
		body, at, err := s.LatestRateLimits("claude", tc.account)
		if err != nil {
			t.Fatalf("latest for %d: %v", tc.account, err)
		}
		if body != tc.want {
			t.Errorf("account %d remembered %q, want %q", tc.account, body, tc.want)
		}
		if at == 0 {
			t.Errorf("account %d reading has no timestamp", tc.account)
		}
	}
}
