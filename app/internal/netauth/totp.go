// Package netauth is the lock on the exposed server: a password and a
// six-digit code in front of the extra listener app/internal/netserver puts
// on the network. Nothing here touches the desktop window's own loopback
// connection, which stays as open as it always was — see
// specs/043-exposed-server.md.
package netauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// The authenticator contract every app implements: HMAC-SHA1 over a 30
// second counter, truncated to six digits. RFC 6238 allows other digests and
// lengths, but no widely used app offers them in a QR, so offering them here
// would only be a way to pair with nothing.
const (
	Period = 30 * time.Second
	digits = 6

	// skew is how many steps either side of now are still accepted, for a
	// phone whose clock has drifted. One step each way is what Google
	// Authenticator and Authy themselves allow.
	skew = 1

	// secretBytes is the length of a generated secret: 160 bits, the size
	// RFC 4226 recommends and the size every authenticator expects.
	secretBytes = 20

	// minSecretBytes is the shortest secret accepted from a human typing one
	// in — 80 bits, RFC 4226's floor.
	minSecretBytes = 10
)

// b32 is the alphabet authenticator apps read: RFC 4648 base32, unpadded,
// which is what an otpauth:// URI carries.
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a fresh random secret, in the unpadded base32 an
// authenticator app expects.
func NewSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return b32.EncodeToString(buf), nil
}

// NormalizeSecret accepts a secret the way a person hands one over — lower
// case, split into spaced groups, padded with '=' by whatever produced it —
// and returns the canonical form to store, or an error naming what is wrong
// with it.
func NormalizeSecret(s string) (string, error) {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(s))
	s = strings.TrimRight(s, "=")
	if s == "" {
		return "", fmt.Errorf("the seed is empty")
	}
	key, err := b32.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("the seed is not base32: only A-Z and 2-7")
	}
	if len(key) < minSecretBytes {
		return "", fmt.Errorf("the seed is too short: %d characters at least", b32.EncodedLen(minSecretBytes))
	}
	return s, nil
}

// code returns the six digits secret stands at for the given counter step.
func code(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// RFC 4226 dynamic truncation: the low nibble of the last byte picks
	// which four bytes of the digest the number is read from.
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", digits, n%1_000_000)
}

// Code is what the authenticator shows for secret at time t. Exported for
// the tests and for nothing else — the app never needs to produce a code,
// only to check one.
func Code(secret string, t time.Time) (string, error) {
	key, err := b32.DecodeString(secret)
	if err != nil {
		return "", err
	}
	return code(key, uint64(t.Unix())/uint64(Period.Seconds())), nil
}

// Verify reports whether entered is the code for secret at time t, allowing
// one step of clock drift either way, and returns the counter it matched.
// The caller uses that counter to refuse a code a second time: a six-digit
// number is good for a whole minute here, which is a whole minute for
// somebody reading it over a shoulder to type it in too.
//
// The comparison is constant-time. Only 10^6 codes exist, so a timing
// channel on the digits is worth rather more than usual.
func Verify(secret, entered string, t time.Time) (uint64, bool) {
	key, err := b32.DecodeString(secret)
	if err != nil || len(key) < minSecretBytes {
		return 0, false
	}
	entered = strings.TrimSpace(entered)
	if len(entered) != digits {
		return 0, false
	}
	now := uint64(t.Unix()) / uint64(Period.Seconds())
	for i := -skew; i <= skew; i++ {
		counter := now + uint64(i)
		if subtle.ConstantTimeCompare([]byte(code(key, counter)), []byte(entered)) == 1 {
			return counter, true
		}
	}
	return 0, false
}

// URI is the otpauth:// link an authenticator app pairs from, as text and as
// the contents of the QR the Server pane draws. issuer appears twice on
// purpose: once in the label and once as a parameter, which is what makes
// both old and new apps file the entry under "agenttik".
func URI(secret, issuer, account string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{
		"secret": {secret},
		"issuer": {issuer},
		"digits": {fmt.Sprint(digits)},
		"period": {fmt.Sprint(int(Period.Seconds()))},
	}
	return "otpauth://totp/" + label + "?" + q.Encode()
}
