package secure

import (
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func newTestSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignerRoundTrip(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	token := s.Sign("42", time.Hour, now)
	got, ok := s.Verify(token, now.Add(time.Minute))
	if !ok || got != "42" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestSignerKeepsPipeInValue(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	got, ok := s.Verify(s.Sign("a|b|c", time.Hour, now), now)
	if !ok || got != "a|b|c" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestSignerRejectsExpired(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	token := s.Sign("42", time.Hour, now)
	if _, ok := s.Verify(token, now.Add(time.Hour)); ok {
		t.Fatal("token accepted at its expiry instant")
	}
}

func TestSignerRejectsTampering(t *testing.T) {
	s := newTestSigner(t)
	now := time.Unix(1_000_000, 0)
	token := s.Sign("42", time.Hour, now)
	body, sig, _ := strings.Cut(token, ".")

	other, _ := NewSigner([]byte("ffffffffffffffffffffffffffffffff"))
	cases := map[string]string{
		"flipped body":  "A" + body[1:] + "." + sig,
		"flipped sig":   body + "." + "A" + sig[1:],
		"no separator":  body,
		"empty":         "",
		"garbage":       "%%%.%%%",
		"other secret":  other.Sign("42", time.Hour, now),
		"signature cut": body + ".",
	}
	for name, tok := range cases {
		if _, ok := s.Verify(tok, now); ok {
			t.Errorf("%s: token accepted", name)
		}
	}
}

func TestNewSignerRejectsShortSecret(t *testing.T) {
	if _, err := NewSigner([]byte("short")); err == nil {
		t.Fatal("expected error for short secret")
	}
}
