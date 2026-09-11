package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/pausan/agenttik/app/internal/agent"
	"github.com/pausan/agenttik/app/internal/store"
)

// Subscriptions, the selectable half. One machine can be signed in to a
// company account and a personal one for the same provider; each is an alias
// and the directory its CLI keeps that login in. What agenttik stores is the
// alias and the path — never the credential, which stays where the CLI put
// it. See 050-subscription-accounts.md.

// accountInfo is one selectable subscription. System marks the account with
// no row of its own: the CLI as the machine already has it, which is what
// every task runs on until another is chosen.
type accountInfo struct {
	ID        int64  `json:"id"`
	Alias     string `json:"alias"`
	Home      string `json:"home"`
	IsDefault bool   `json:"is_default"`
	System    bool   `json:"system"`

	// SignedIn and Detail are read from the login directory itself, so the
	// list says which accounts are ready without asking a CLI to start.
	SignedIn bool   `json:"signed_in"`
	Detail   string `json:"detail,omitempty"`
}

// systemAlias is what the CLI's own login is called on screen. Named rather
// than blank because it appears beside the others in every picker.
const systemAlias = "System"

// providerAccounts is what one provider offers: the system account first,
// then the configured ones in the order they were added. A provider that
// cannot hold a second login offers the system account alone, so the UI has
// one shape to draw either way.
func (s *Server) providerAccounts(p agent.Provider, rows []store.Account) []accountInfo {
	multi, ok := p.(agent.MultiAccount)
	system := accountInfo{ID: store.SystemAccount, Alias: systemAlias, System: true, IsDefault: true}
	if !ok {
		return []accountInfo{system}
	}
	system.Home = multi.DefaultHome()
	status := multi.AccountStatus("")
	system.SignedIn, system.Detail = status.SignedIn, status.Detail

	out := []accountInfo{system}
	for _, row := range rows {
		if row.Provider != p.Name() {
			continue
		}
		status := multi.AccountStatus(row.Home)
		if row.IsDefault {
			out[0].IsDefault = false
		}
		out = append(out, accountInfo{
			ID: row.ID, Alias: row.Alias, Home: row.Home, IsDefault: row.IsDefault,
			SignedIn: status.SignedIn, Detail: status.Detail,
		})
	}
	return out
}

// account resolves one subscription of one provider from the path. It is the
// single gate for "does this account exist, and is it this provider's": a
// mismatch here would run a prompt on the wrong allowance.
func (s *Server) account(c *fiber.Ctx) (agent.Provider, *store.Account, error) {
	name := c.Params("provider")
	provider, ok := s.registry.Get(name)
	if !ok {
		return nil, nil, badRequest("unknown provider %q", name)
	}
	id, err := strconv.ParseInt(c.Params("account"), 10, 64)
	if err != nil {
		return nil, nil, badRequest("invalid subscription id %q", c.Params("account"))
	}
	if id == store.SystemAccount {
		return provider, nil, nil // the CLI's own login; there is no row to read
	}
	account, err := s.store.GetAccount(id)
	if err != nil {
		return nil, nil, err
	}
	if account.Provider != name {
		return nil, nil, badRequest("subscription %q does not belong to %s", account.Alias, name)
	}
	return provider, account, nil
}

// accountHome is the login directory for an account id already known to
// belong to the provider, empty for the system one.
func accountHome(account *store.Account) string {
	if account == nil {
		return ""
	}
	return account.Home
}

// checkMultiAccount refuses a second subscription for a provider whose login
// cannot be pointed anywhere else. Better to say so than to store a path the
// CLI will ignore.
func checkMultiAccount(p agent.Provider) (agent.MultiAccount, error) {
	multi, ok := p.(agent.MultiAccount)
	if !ok {
		return nil, badRequest("%s cannot hold more than one subscription", p.DisplayName())
	}
	return multi, nil
}

// chooseAccount resolves which subscription a task runs on when its provider,
// model or effort is set.
//
// A request that names one is taken at its word once it has been checked
// against the provider. A request that changes provider without naming one
// lands on that provider's default, because the account it had belongs to the
// provider it just left — carrying the number across would point at another
// provider's subscription, or at nothing.
func (s *Server) chooseAccount(providerName string, requested *int64, current int64, providerChanged bool) (int64, error) {
	if requested != nil {
		if *requested == store.SystemAccount {
			return store.SystemAccount, nil
		}
		account, err := s.store.GetAccount(*requested)
		if err != nil {
			return 0, err
		}
		if account.Provider != providerName {
			return 0, badRequest("subscription %q does not belong to %s", account.Alias, providerName)
		}
		return account.ID, nil
	}
	if providerChanged {
		return s.store.DefaultAccount(providerName)
	}
	return current, nil
}

type accountBody struct {
	Alias string `json:"alias"`
	Home  string `json:"home"`
	// Default asks for this to be what new tasks start on, so adding a
	// subscription and switching to it is one request.
	Default bool `json:"default"`
}

// createAccount adds a subscription. Only the alias is required: a home left
// out is one this app manages, beside the database, named after the alias —
// which is the common case, since the point is a second login rather than a
// particular place to keep it.
func (s *Server) createAccount(c *fiber.Ctx) error {
	name := c.Params("provider")
	provider, ok := s.registry.Get(name)
	if !ok {
		return badRequest("unknown provider %q", name)
	}
	if _, err := checkMultiAccount(provider); err != nil {
		return err
	}
	var body accountBody
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	alias := strings.TrimSpace(body.Alias)
	if alias == "" {
		return badRequest("a name for the subscription is required")
	}
	if strings.EqualFold(alias, systemAlias) {
		return badRequest("%q is what the CLI's own login is called; choose another name", systemAlias)
	}
	home := strings.TrimSpace(body.Home)
	if home == "" {
		home = s.managedHome(name, alias)
	}
	if !filepath.IsAbs(home) {
		return badRequest("the login directory must be an absolute path")
	}
	account := &store.Account{Provider: name, Alias: alias, Home: home, IsDefault: body.Default}
	if err := s.store.CreateAccount(account); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return badRequest("%s already has a subscription called %q", provider.DisplayName(), alias)
		}
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(s.accountResponse(provider, account))
}

// updateAccount renames a subscription or repoints it at another directory.
// The system account has no row: it can be made the default, which is the
// endpoint below, but it cannot be renamed or moved from here.
func (s *Server) updateAccount(c *fiber.Ctx) error {
	provider, account, err := s.account(c)
	if err != nil {
		return err
	}
	if account == nil {
		return badRequest("the CLI's own login cannot be renamed or moved")
	}
	var body accountBody
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if alias := strings.TrimSpace(body.Alias); alias != "" {
		if strings.EqualFold(alias, systemAlias) {
			return badRequest("%q is what the CLI's own login is called; choose another name", systemAlias)
		}
		account.Alias = alias
	}
	if home := strings.TrimSpace(body.Home); home != "" {
		if !filepath.IsAbs(home) {
			return badRequest("the login directory must be an absolute path")
		}
		account.Home = home
	}
	if err := s.store.UpdateAccount(account); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return badRequest("%s already has a subscription called %q", provider.DisplayName(), account.Alias)
		}
		return err
	}
	return c.JSON(s.accountResponse(provider, account))
}

// deleteAccount forgets a subscription. The login directory is left on disk:
// it holds an account this app did not create, and removing a row here is not
// a reason to sign anything out. Tasks still pointing at it keep saying so
// and refuse to start until another subscription is chosen for them, rather
// than quietly moving to someone else's allowance.
func (s *Server) deleteAccount(c *fiber.Ctx) error {
	_, account, err := s.account(c)
	if err != nil {
		return err
	}
	if account == nil {
		return badRequest("the CLI's own login cannot be removed")
	}
	if err := s.store.DeleteAccount(account.ID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// accountUsage is what still points at a subscription, which is what the
// confirmation before removing one says out loud. Asked only when removing,
// so listing subscriptions stays two queries rather than two per row.
func (s *Server) accountUsage(c *fiber.Ctx) error {
	_, account, err := s.account(c)
	if err != nil {
		return err
	}
	if account == nil {
		return c.JSON(fiber.Map{"sessions": 0, "schedules": 0})
	}
	sessions, schedules, err := s.store.AccountUsage(account.ID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"sessions": sessions, "schedules": schedules})
}

// setDefaultAccount chooses which subscription a new task on this provider
// starts on. 0 is the CLI's own login, which is the default until something
// else is named — so this is also how the choice is undone.
func (s *Server) setDefaultAccount(c *fiber.Ctx) error {
	name := c.Params("provider")
	provider, ok := s.registry.Get(name)
	if !ok {
		return badRequest("unknown provider %q", name)
	}
	var body struct {
		AccountID int64 `json:"account_id"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid body: %v", err)
	}
	if body.AccountID != store.SystemAccount {
		account, err := s.store.GetAccount(body.AccountID)
		if err != nil {
			return err
		}
		if account.Provider != name {
			return badRequest("subscription %q does not belong to %s", account.Alias, name)
		}
	}
	if err := s.store.SetDefaultAccount(name, body.AccountID); err != nil {
		return err
	}
	rows, err := s.store.ListAccounts()
	if err != nil {
		return err
	}
	return c.JSON(s.providerAccounts(provider, rows))
}

// loginAccount hands the CLI's own sign-in over to a terminal, with the
// account's directory already set.
//
// It is handed over rather than driven: all three CLIs render their login on
// a terminal and answer keypresses, and none of them prints the URL when its
// output is a pipe, so there is nothing for a browser to relay. The command
// is always returned — a machine with no terminal to open, or a UI being
// read from another one, can still be told exactly what to run.
func (s *Server) loginAccount(c *fiber.Ctx) error {
	provider, account, err := s.account(c)
	if err != nil {
		return err
	}
	multi, err := checkMultiAccount(provider)
	if err != nil {
		return err
	}
	home := accountHome(account)
	if home != "" {
		// The CLI creates this itself, but only once it can chdir into it, and
		// 0700 is what a directory about to hold a credential should be.
		if err := os.MkdirAll(home, 0o700); err != nil {
			return badRequest("cannot prepare %s: %v", home, err)
		}
	}
	login := multi.LoginCommand(home)
	shell := shellLine(login)
	spawned := true
	if err := openTerminal(login); err != nil {
		spawned = false
	}
	return c.JSON(fiber.Map{"command": shell, "spawned": spawned})
}

// accountResponse is one row as the UI wants it back: what was saved, plus
// whether that directory is signed in.
func (s *Server) accountResponse(p agent.Provider, account *store.Account) accountInfo {
	info := accountInfo{ID: account.ID, Alias: account.Alias, Home: account.Home,
		IsDefault: account.IsDefault}
	if multi, ok := p.(agent.MultiAccount); ok {
		status := multi.AccountStatus(account.Home)
		info.SignedIn, info.Detail = status.SignedIn, status.Detail
	}
	return info
}

// managedHome is where a subscription's login goes when the user did not say:
// beside the database, under the provider and a readable form of the alias.
// Kept out of the CLI's own directory so signing this one out, or deleting
// it, can never touch the account the machine was already using.
func (s *Server) managedHome(provider, alias string) string {
	return filepath.Join(s.store.Dir(), "accounts", provider, slug(alias))
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug makes a directory name out of an alias. An alias of nothing but
// punctuation would leave it empty, so the uniqueness the alias already has
// is borrowed rather than invented: the caller's alias is unique per
// provider, and "account" plus that is a valid name for the odd case.
func slug(alias string) string {
	name := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(alias), "-"), "-")
	if name == "" {
		return "account"
	}
	return name
}

// shellLine renders a login command the way a human would type it, which is
// what the UI shows beside the button for anyone who would rather run it
// themselves — or has no desktop terminal to open.
func shellLine(cmd agent.LoginCommand) string {
	parts := make([]string, 0, len(cmd.Env)+len(cmd.Args))
	for _, env := range cmd.Env {
		if key, value, ok := strings.Cut(env, "="); ok {
			parts = append(parts, key+"="+shellQuote(value))
		}
	}
	for _, arg := range cmd.Args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

// shellQuote is single-quote quoting: everything inside is literal, and the
// only character needing care is the quote itself.
func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\"'$&|;<>()*?[]{}#~!\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// loginHold keeps the terminal open after the CLI exits, so a login that
// failed says why instead of vanishing with the window.
const loginHold = `; printf '\nPress Enter to close…'; read _`
