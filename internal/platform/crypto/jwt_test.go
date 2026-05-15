package crypto

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func mustSigner(t *testing.T) *JWTSigner {
	t.Helper()
	secret := bytes.Repeat([]byte{0xab}, MinJWTSecretLength)
	signer, err := NewJWTSigner(secret, "observai-api")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer
}

func TestNewJWTSignerRejectsShortSecret(t *testing.T) {
	if _, err := NewJWTSigner(bytes.Repeat([]byte{1}, MinJWTSecretLength-1), ""); !errors.Is(err, ErrInvalidJWTSecret) {
		t.Fatalf("expected ErrInvalidJWTSecret, got %v", err)
	}
}

func TestJWTSignerSignParseRoundTrip(t *testing.T) {
	signer := mustSigner(t)
	now := time.Now().UTC().Truncate(time.Second)
	claims := JWTClaims{
		Subject:   "user-1",
		Role:      "admin",
		JTI:       "tok-1",
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
	}
	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Fatalf("token does not look like a jwt: %q", token)
	}
	parsed, err := signer.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Subject != claims.Subject || parsed.Role != claims.Role || parsed.JTI != claims.JTI {
		t.Fatalf("claims mismatch: %+v vs %+v", parsed, claims)
	}
	if !parsed.ExpiresAt.Equal(claims.ExpiresAt) {
		t.Fatalf("expires mismatch: %v vs %v", parsed.ExpiresAt, claims.ExpiresAt)
	}
}

func TestJWTSignerRejectsForeignSignature(t *testing.T) {
	signerOne := mustSigner(t)
	otherSecret := bytes.Repeat([]byte{0xcd}, MinJWTSecretLength)
	signerTwo, _ := NewJWTSigner(otherSecret, "observai-api")
	token, _ := signerOne.Sign(JWTClaims{
		Subject:   "u",
		Role:      "viewer",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if _, err := signerTwo.Parse(token); !errors.Is(err, ErrInvalidJWT) {
		t.Fatalf("expected ErrInvalidJWT, got %v", err)
	}
}

func TestJWTSignerRejectsExpiredToken(t *testing.T) {
	signer := mustSigner(t)
	expired := time.Now().Add(-time.Hour)
	token, _ := signer.Sign(JWTClaims{
		Subject:   "u",
		Role:      "viewer",
		IssuedAt:  expired.Add(-time.Minute),
		ExpiresAt: expired,
	})
	if _, err := signer.Parse(token); !errors.Is(err, ErrInvalidJWT) {
		t.Fatalf("expected ErrInvalidJWT for expired token, got %v", err)
	}
}

func TestJWTSignerRejectsIssuerMismatch(t *testing.T) {
	signer := mustSigner(t)
	other, _ := NewJWTSigner(bytes.Repeat([]byte{0xab}, MinJWTSecretLength), "other-service")
	token, _ := other.Sign(JWTClaims{
		Subject:   "u",
		Role:      "viewer",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if _, err := signer.Parse(token); !errors.Is(err, ErrInvalidJWT) {
		t.Fatalf("expected ErrInvalidJWT for issuer mismatch, got %v", err)
	}
}
