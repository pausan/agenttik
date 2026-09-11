package netauth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"rsc.io/qr"
)

// MinPasswordLen is the shortest password accepted. The lock is meant for a
// server on a home or office network rather than the open internet, and a
// second factor stands behind it, so this is a floor against "1234" rather
// than a policy.
const MinPasswordLen = 8

// HashPassword returns what gets stored for a password. bcrypt's default
// cost is about a tenth of a second on a current machine, which is the
// throttle under every guess at the login form.
func HashPassword(plain string) (string, error) {
	if len(plain) < MinPasswordLen {
		return "", fmt.Errorf("the password must be at least %d characters", MinPasswordLen)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(h), nil
}

// QRPNG draws uri as the QR an authenticator app scans. Level M survives a
// phone camera on a screen, and keeps the image small enough to sit in a
// settings pane.
func QRPNG(uri string) ([]byte, error) {
	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		return nil, fmt.Errorf("encode qr: %w", err)
	}
	return code.PNG(), nil
}
