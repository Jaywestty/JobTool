package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

// Token is an opaque session token; only its hash is stored.
type Token struct {
	Raw       string
	Hash      []byte
	ExpiresAt time.Time
}

// NewToken issues a session token valid for ttl.
func NewToken(ttl time.Duration) (Token, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return Token{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return Token{Raw: raw, Hash: HashToken(raw), ExpiresAt: time.Now().Add(ttl)}, nil
}

// HashToken hashes a raw token exactly as it is stored in sessions.token_hash.
func HashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// NormalizeEmail lowercases and trims an email for uniqueness checks.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// ValidateEmail is a deliberately small syntax check; deliverability is not
// this package's job.
func ValidateEmail(email string) error {
	e := NormalizeEmail(email)
	at := strings.IndexByte(e, '@')
	if at <= 0 || at == len(e)-1 || strings.Contains(e, " ") || strings.Count(e, "@") != 1 {
		return errors.New("invalid email")
	}
	domain := e[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return errors.New("invalid email domain")
	}
	return nil
}
