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

func TestGeneratePassword(t *testing.T) {
	p1, err := auth.GeneratePassword(0) // 0 -> default length
	if err != nil {
		t.Fatalf("GeneratePassword: %v", err)
	}
	if len(p1) < 16 {
		t.Errorf("default password too short: %d chars", len(p1))
	}

	// Two calls must not collide (random source).
	p2, err := auth.GeneratePassword(0)
	if err != nil {
		t.Fatalf("GeneratePassword (2): %v", err)
	}
	if p1 == p2 {
		t.Fatal("two generated passwords are identical — not random")
	}

	// A generated password round-trips through hashing.
	h, err := auth.HashPassword(p1)
	if err != nil {
		t.Fatalf("HashPassword(generated): %v", err)
	}
	if !auth.CheckPassword(h, p1) {
		t.Error("generated password failed to verify against its own hash")
	}
}
