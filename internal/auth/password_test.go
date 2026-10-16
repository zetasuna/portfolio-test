package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == password {
		t.Fatal("password must not be stored as plaintext")
	}
	if !VerifyPassword(hash, password) {
		t.Fatal("password should verify")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatal("wrong password verified")
	}
}
func TestHashPasswordRejectsShortPassword(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected minimum password length")
	}
}
func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if VerifyPassword("not-a-hash", "password") {
		t.Fatal("malformed hash accepted")
	}
}
