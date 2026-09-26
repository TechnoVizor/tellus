package secure

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Signer issues and checks tamper-proof tokens:
// base64url(expiry|value) + "." + base64url(HMAC-SHA256(secret, expiry|value)).
type Signer struct{ secret []byte }

// NewSigner requires a secret of at least 32 bytes.
func NewSigner(secret []byte) (*Signer, error) {
	if len(secret) < 32 {
		return nil, errors.New("secure: signing secret must be at least 32 bytes")
	}
	return &Signer{secret: append([]byte(nil), secret...)}, nil
}

// Sign returns a token carrying value that expires ttl after now.
func (s *Signer) Sign(value string, ttl time.Duration, now time.Time) string {
	payload := strconv.FormatInt(now.Add(ttl).Unix(), 10) + "|" + value
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return body + "." + base64.RawURLEncoding.EncodeToString(s.mac(body))
}

// Verify returns the value of a valid, unexpired token.
func (s *Signer) Verify(token string, now time.Time) (string, bool) {
	body, sig, found := strings.Cut(token, ".")
	if !found {
		return "", false
	}
	gotSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(gotSig, s.mac(body)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", false
	}
	expiry, value, found := strings.Cut(string(raw), "|")
	if !found {
		return "", false
	}
	exp, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || now.Unix() >= exp {
		return "", false
	}
	return value, true
}

func (s *Signer) mac(body string) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(body))
	return m.Sum(nil)
}
