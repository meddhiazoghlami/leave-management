// Package auth handles password hashing, session tokens/cookies, and the Gin
// middleware that turns a session cookie into the current employee. This is the
// one genuinely new concept Phase 9 introduces on top of the assembled stack.
package auth

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword produces a bcrypt hash suitable for storing in
// employees.password_hash. bcrypt salts internally, so equal passwords still
// yield different hashes.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored bcrypt hash. The
// comparison is constant-time within bcrypt.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// GeneratePassword returns a cryptographically-random password with roughly
// bytes*8 bits of entropy, encoded URL-safe (base64 without padding) so it's
// printable and copy-pasteable. Used to mint initial passwords for bootstrap
// accounts, which are mailed to the user and never stored in plaintext.
func GeneratePassword(bytes int) (string, error) {
	if bytes <= 0 {
		bytes = 18 // 18 bytes -> 24-char token, ~144 bits
	}
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
