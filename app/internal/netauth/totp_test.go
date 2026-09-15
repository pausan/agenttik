package netauth

import (
	"testing"
	"time"
)

// rfcSecret is the key RFC 6238's own test vectors use: the ASCII digits
// "12345678901234567890", in the base32 an authenticator would be given.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

// TestCodeMatchesRFC6238 checks the generator against the published vectors,
// truncated to the six digits every authenticator app shows.
func TestCodeMatchesRFC6238(t *testing.T) {
	for _, tc := range []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		got, err := Code(rfcSecret, time.Unix(tc.unix, 0))
		if err != nil {
			t.Fatalf("Code at %d: %v", tc.unix, err)
		}
		if got != tc.want {
			t.Errorf("Code at %d = %s, want %s", tc.unix, got, tc.want)
		}
	}
}

// TestVerifyAcceptedWindows checks the exact two-window validity range.
func TestVerifyAcceptedWindows(t *testing.T) {
	now := time.Unix(1234567890, 0)
	for _, offset := range []time.Duration{-Period, 0} {
		code, err := Code(rfcSecret, now.Add(offset))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := Verify(rfcSecret, code, now); !ok {
			t.Errorf("code %s from %v away was rejected", code, offset)
		}
	}
	for _, offset := range []time.Duration{-2 * Period, Period, 2 * Period} {
		code, _ := Code(rfcSecret, now.Add(offset))
		if _, ok := Verify(rfcSecret, code, now); ok {
			t.Errorf("code %s from %v away was accepted", code, offset)
		}
	}
}

// TestVerifyReportsItsStep checks that the counter handed back is the one
// that matched, which is what the gate refuses to accept a second time.
func TestVerifyReportsItsStep(t *testing.T) {
	now := time.Unix(1234567890, 0)
	code, _ := Code(rfcSecret, now.Add(-Period))
	step, ok := Verify(rfcSecret, code, now)
	if !ok {
		t.Fatal("the previous step's code was rejected")
	}
	if want := uint64(now.Unix())/30 - 1; step != want {
		t.Errorf("step = %d, want %d", step, want)
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	now := time.Unix(1234567890, 0)
	good, _ := Code(rfcSecret, now)
	for name, code := range map[string]string{
		"empty":     "",
		"too short": good[:5],
		"too long":  good + "0",
		"letters":   "abcdef",
	} {
		if _, ok := Verify(rfcSecret, code, now); ok {
			t.Errorf("%s (%q) was accepted", name, code)
		}
	}
	// A secret too short to be a key opens nothing, whatever is typed at it.
	if _, ok := Verify("GEZDGNBV", good, now); ok {
		t.Error("an 80-bit-short secret accepted a code")
	}
}

// TestNewSecretIsUsable checks a generated seed round-trips through the same
// path an authenticator app would take it down.
func TestNewSecretIsUsable(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := NormalizeSecret(secret); err != nil || got != secret {
		t.Fatalf("NormalizeSecret(%q) = %q, %v; want it unchanged", secret, got, err)
	}
	now := time.Now()
	code, err := Code(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := Verify(secret, code, now); !ok {
		t.Error("a fresh secret did not verify its own code")
	}

	other, _ := NewSecret()
	if other == secret {
		t.Error("two generated secrets were the same")
	}
}

// TestNormalizeSecret covers the shapes a person actually pastes in.
func TestNormalizeSecret(t *testing.T) {
	for in, want := range map[string]string{
		"gezdgnbvgy3tqojq":       "GEZDGNBVGY3TQOJQ",
		"GEZD GNBV GY3T QOJQ":    "GEZDGNBVGY3TQOJQ",
		"GEZD-GNBV-GY3T-QOJQ":    "GEZDGNBVGY3TQOJQ",
		"GEZDGNBVGY3TQOJQ======": "GEZDGNBVGY3TQOJQ",
	} {
		got, err := NormalizeSecret(in)
		if err != nil {
			t.Errorf("NormalizeSecret(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeSecret(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "   ", "GEZDGNBV", "not-base32-at-all!", "01890"} {
		if _, err := NormalizeSecret(bad); err == nil {
			t.Errorf("NormalizeSecret(%q) was accepted", bad)
		}
	}
}

func TestURICarriesThePairing(t *testing.T) {
	uri := URI(rfcSecret, "agenttik", "desktop")
	for _, want := range []string{
		"otpauth://totp/agenttik:desktop?",
		"secret=" + rfcSecret,
		"issuer=agenttik",
		"digits=6",
		"period=30",
	} {
		if !contains(uri, want) {
			t.Errorf("URI = %q, missing %q", uri, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
