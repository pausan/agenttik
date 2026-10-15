package netauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// ChallengePath hands out a single-use nonce, and KeyPath takes it back
	// signed by a trusted machine's key in exchange for a session.
	ChallengePath = prefix + "challenge"
	KeyPath       = prefix + "key"

	// nonceTTL is how long a challenge may wait for its signature, and
	// maxNonces how many may wait at once, so a script asking for nonces
	// cannot grow the map without bound.
	nonceTTL  = time.Minute
	maxNonces = 1024
)

// Machine is another agenttik that signed in here with the password (and the
// code, when 2FA is on) and is trusted from then on by its public key, from
// whatever address it reaches this machine.
type Machine struct {
	Key     string // base64url Ed25519 public key
	ID      string // the client's machine ID, for display
	Name    string // the client's server name, for display
	Address string // where it last connected from
}

// Trust is the list of trusted machines. Visit reports whether key is on it,
// recording the visit when it is; Add puts a machine on it after a full
// sign-in, or refreshes the entry already there.
type Trust interface {
	Visit(m Machine) bool
	Add(m Machine) error
}

// UseTrust lets machines on t sign in with their key. serverID is this
// machine's ID, which every signature covers, so a signature made for one
// server cannot be replayed against another. Call it before serving.
func (g *Gate) UseTrust(serverID string, t Trust) {
	g.serverID, g.trust = serverID, t
}

// KeyMessage is what a client signs to sign in: the server it means and the
// nonce that server handed out.
func KeyMessage(serverID, nonce string) []byte {
	return []byte("agenttik-key-login-v1\n" + serverID + "\n" + nonce)
}

// challengeKey hands out a fresh nonce for a key sign-in.
func (g *Gate) challengeKey(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	if g.trust == nil || g.serverID == "" {
		writeJSONError(w, http.StatusNotFound, "key sign-in is not available here")
		return
	}
	nonce, err := newToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not make a challenge")
		return
	}
	now := time.Now()
	g.mu.Lock()
	for k, until := range g.nonces {
		if now.After(until) {
			delete(g.nonces, k)
		}
	}
	full := len(g.nonces) >= maxNonces
	if !full {
		g.nonces[nonce] = now.Add(nonceTTL)
	}
	g.mu.Unlock()
	if full {
		writeJSONError(w, http.StatusServiceUnavailable, "too many pending sign-ins")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"nonce": nonce}) //nolint:errcheck // client is gone
}

// loginKey starts a session for a trusted machine that signed a nonce. A
// refusal is not counted against the address: a forged signature is not a
// guess that gets closer, and a machine removed from the list would otherwise
// lock its owner out of the password form while it keeps retrying.
func (g *Gate) loginKey(w http.ResponseWriter, r *http.Request, creds Credentials) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var body struct {
		Key       string `json:"key"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
		ID        string `json:"id"`
		Name      string `json:"name"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid key sign-in")
		return
	}
	if g.trust == nil || g.serverID == "" || !creds.ready() {
		writeJSONError(w, http.StatusUnauthorized, "key sign-in is not available here")
		return
	}
	g.mu.Lock()
	until, ok := g.nonces[body.Nonce]
	delete(g.nonces, body.Nonce)
	g.mu.Unlock()
	if !ok || time.Now().After(until) {
		writeJSONError(w, http.StatusUnauthorized, "the challenge expired")
		return
	}
	pub, err := ParseKey(body.Key)
	sig, sigErr := base64.RawURLEncoding.DecodeString(body.Signature)
	if err != nil || sigErr != nil || !ed25519.Verify(pub, KeyMessage(g.serverID, body.Nonce), sig) {
		writeJSONError(w, http.StatusUnauthorized, "the signature does not match")
		return
	}
	if !g.trust.Visit(machineFrom(body.Key, body.ID, body.Name, r)) {
		writeJSONError(w, http.StatusUnauthorized, "this machine is not trusted here")
		return
	}
	if err := g.startSession(w, body.Key); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// remember trusts the machine a password sign-in came from, when it named its
// key. A browser names none and is never trusted. A malformed key is ignored
// rather than failing a sign-in that was otherwise right.
func (g *Gate) remember(r *http.Request) string {
	key := r.FormValue("key")
	if key == "" || g.trust == nil {
		return ""
	}
	if _, err := ParseKey(key); err != nil {
		return ""
	}
	if err := g.trust.Add(machineFrom(key, r.FormValue("id"), r.FormValue("name"), r)); err != nil {
		return ""
	}
	return key
}

// RevokeKey ends the sessions a machine opened, as it is removed from the
// trusted list.
func (g *Gate) RevokeKey(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for token, s := range g.sessions {
		if s.key == key {
			delete(g.sessions, token)
		}
	}
}

func machineFrom(key, id, name string, r *http.Request) Machine {
	return Machine{Key: key, ID: clip(id, 64), Name: clip(name, 100), Address: clientAddr(r)}
}

// clip keeps a display field short and valid UTF-8.
func clip(s string, n int) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "")
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck // client is gone
}

// ParseKey decodes a public key as it travels and is stored.
func ParseKey(key string) (ed25519.PublicKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("not an Ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

// EncodeKey is the inverse of ParseKey.
func EncodeKey(pub ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(pub)
}

// Sign signs a server's nonce with this machine's key.
func Sign(key ed25519.PrivateKey, serverID, nonce string) string {
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, KeyMessage(serverID, nonce)))
}

// Fingerprint is a short, stable label for a key, for telling entries apart.
func Fingerprint(key string) string {
	b, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(b) < 8 {
		return key
	}
	return hex.EncodeToString(b[:8])
}

// LoadOrCreateKey reads this machine's private key from path, making one
// the first time it is asked for. The file is PKCS#8 PEM, mode 0600, and is
// written by atomic rename so a crash never leaves half a key behind.
func LoadOrCreateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("read %s: not a PEM key", path)
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		priv, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("read %s: not an Ed25519 key", path)
		}
		return priv, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err == nil {
		err = pem.Encode(tmp, &pem.Block{Type: "PRIVATE KEY", Bytes: der})
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, err
	}
	return priv, nil
}
