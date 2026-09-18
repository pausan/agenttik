package netauth

import (
	_ "embed"
	"html/template"
	"log"
	"net/http"
)

// The form an unauthenticated browser gets, whatever it asked for. It is one
// self-contained file on purpose: the UI's own bundle lives behind the gate,
// so nothing of the app — not a script, not an asset, not the list of
// projects in the page title — is served to somebody who has not signed in.
//
//go:embed login.html
var loginHTML string

var loginPage = template.Must(template.New("login").Parse(loginHTML))

type loginData struct {
	Why         string // what went wrong last time, blank on a first draw
	Next        string // where a good login lands
	TOTPEnabled bool
	Ready       bool // whether a password and a seed actually exist to check against
}

func writeLoginPage(w http.ResponseWriter, status int, why, next string, creds Credentials) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A login form is per-session and must never come back out of a cache,
	// least of all a shared one on the way here.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := loginPage.Execute(w, loginData{Why: why, Next: next, Ready: creds.ready(), TOTPEnabled: !creds.TOTPDisabled}); err != nil {
		log.Printf("netauth: write login page: %v", err)
	}
}
