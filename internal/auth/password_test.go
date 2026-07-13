package auth_test

import (
	"testing"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
)

func TestHashAndCheckPassword(t *testing.T) {
	const plain = "correct horse battery staple"

	h1, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h1 == plain {
		t.Fatal("hash equals plaintext — not hashed")
	}

	// bcrypt salts internally: equal passwords hash differently...
	h2, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword (2): %v", err)
	}
	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical — missing salt")
	}

	// ...yet both verify against the original.
	if !auth.CheckPassword(h1, plain) {
		t.Error("CheckPassword rejected the correct password (h1)")
	}
	if !auth.CheckPassword(h2, plain) {
		t.Error("CheckPassword rejected the correct password (h2)")
	}

	// Wrong password fails.
	if auth.CheckPassword(h1, "wrong") {
		t.Error("CheckPassword accepted a wrong password")
	}
	// A non-bcrypt hash fails rather than panicking.
	if auth.CheckPassword("not-a-bcrypt-hash", plain) {
		t.Error("CheckPassword accepted a malformed hash")
	}
}
