package netauth

import (
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestPasswordHashesHaveIndependentSalts(t *testing.T) {
	const password = "correct horse battery"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == password {
		t.Fatal("password hashes must contain independent random salts")
	}
	for _, hash := range []string{first, second} {
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
			t.Fatal(err)
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong password")) == nil {
			t.Fatal("wrong password accepted")
		}
	}
}
