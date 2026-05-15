package crypto

import (
	"errors"
	"testing"
)

func TestHashPasswordVerifyRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "" {
		t.Fatalf("hash must not be empty")
	}
	if err := VerifyPassword(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("verify password: %v", err)
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	hash, _ := HashPassword("right", 4)
	err := VerifyPassword(hash, "wrong")
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected ErrPasswordMismatch, got %v", err)
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	err := VerifyPassword("not-a-real-bcrypt-hash", "x")
	if err == nil {
		t.Fatalf("expected error for malformed hash")
	}
	if errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("malformed hash must not surface as mismatch")
	}
}

func TestHashPasswordUsesDefaultCostForNonPositive(t *testing.T) {
	hash, err := HashPassword("x", 0)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "" {
		t.Fatalf("hash must not be empty")
	}
}
