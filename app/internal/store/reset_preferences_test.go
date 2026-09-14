package store

import "testing"

func TestResetPreferencesPreservesAccountsAndWork(t *testing.T) {
	s := testStore(t)
	for _, q := range []string{
		`UPDATE general_config SET new_item_position = 'bottom'`,
		`UPDATE desktop_config SET close_to_tray = 1, toggle_shortcut = 'Alt+B'`,
		`INSERT INTO provider_accounts (provider, alias, home, is_default, created_at) VALUES ('test', 'personal', '/account', 1, 1)`,
		`INSERT INTO starred_models (provider, model, created_at) VALUES ('test', 'model', 1)`,
		`INSERT INTO hidden_model_choices (provider, model) VALUES ('test', 'model')`,
		`INSERT INTO server_config (id, password_hash, totp_secret, auth_enabled) VALUES (1, 'keep-password', 'keep-totp', 1)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.CreateProject("keep", "/keep")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(&Session{ID: "keep", ProjectID: p.ID, Provider: "test", Model: "model"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.ResetPreferences(); err != nil {
			t.Fatal(err)
		}
		for query, want := range map[string]string{
			`SELECT new_item_position FROM general_config`:  "top",
			`SELECT toggle_shortcut FROM desktop_config`:    "Ctrl+Shift+A",
			`SELECT close_to_tray FROM desktop_config`:      "0",
			`SELECT COUNT(*) FROM starred_models`:           "0",
			`SELECT COUNT(*) FROM hidden_model_choices`:     "0",
			`SELECT home FROM provider_accounts`:            "/account",
			`SELECT is_default FROM provider_accounts`:      "0",
			`SELECT password_hash FROM server_config`:       "keep-password",
			`SELECT totp_secret FROM server_config`:         "keep-totp",
			`SELECT auth_enabled FROM server_config`:        "1",
			`SELECT name FROM projects WHERE name = 'keep'`: "keep",
			`SELECT id FROM sessions WHERE id = 'keep'`:     "keep",
		} {
			var got string
			if err := s.db.QueryRow(query).Scan(&got); err != nil {
				t.Fatalf("%s: %v", query, err)
			}
			if got != want {
				t.Fatalf("%s = %q, want %q", query, got, want)
			}
		}
	}
}
