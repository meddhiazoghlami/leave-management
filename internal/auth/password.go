// Package auth handles password hashing, session tokens/cookies, and the Gin
// middleware that turns a session cookie into the current employee. This is the
// one genuinely new concept Phase 9 introduces on top of the assembled stack.
package auth

import "golang.org/x/crypto/bcrypt"

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
