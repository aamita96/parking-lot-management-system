package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("s3cret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("s3cret-pass", h) {
		t.Error("correct password rejected")
	}
	if VerifyPassword("wrong", h) || VerifyPassword("s3cret-pass", "garbage") {
		t.Error("wrong password accepted")
	}
}

func TestTokens(t *testing.T) {
	tk := NewTokens([]byte("0123456789abcdef0123456789abcdef"))
	s, _, err := tk.Issue("owner-1", RoleOwner, "o@example.com", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c, err := tk.Parse(s, RoleOwner)
	if err != nil || c.Subject != "owner-1" || c.Email != "o@example.com" {
		t.Fatalf("parse failed: %v %+v", err, c)
	}
	if _, err := tk.Parse(s, RoleAdmin); err == nil {
		t.Error("owner token accepted as admin")
	}
	expired, _, _ := tk.Issue("x", RoleAdmin, "", -time.Minute)
	if _, err := tk.Parse(expired, RoleAdmin); err == nil {
		t.Error("expired token accepted")
	}
	other := NewTokens([]byte("ffffffffffffffffffffffffffffffff"))
	if _, err := other.Parse(s, RoleOwner); err == nil {
		t.Error("token with wrong key accepted")
	}
}

func TestCodes(t *testing.T) {
	secret := []byte("code-secret-123456")
	code := NewCode()
	if len(code) != 6 {
		t.Fatalf("code %q not 6 digits", code)
	}
	h := HashCode(secret, "id1", code)
	if !CodeMatches(secret, "id1", code, h) {
		t.Error("code did not match")
	}
	if CodeMatches(secret, "id2", code, h) {
		t.Error("code matched another scope")
	}
	k, kh := NewGateKey()
	if HashGateKey(k) != kh {
		t.Error("gate key hash mismatch")
	}
}
