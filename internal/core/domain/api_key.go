package domain

import (
	"errors"
	"time"
)

// ErrAPIKeyNotFound is returned when a referenced API key does not exist or
// has been revoked.
var ErrAPIKeyNotFound = errors.New("api key not found")

// ErrInvalidAPIKey is returned when API key input fails validation.
var ErrInvalidAPIKey = errors.New("invalid api key")

// APIKeyScope describes the authorization scope granted to a key.
//
// "default" allows access to read/write operational endpoints.
// "admin" additionally allows access to /v1/admin/* management endpoints.
type APIKeyScope string

const (
	// APIKeyScopeDefault grants access to the application endpoints.
	APIKeyScopeDefault APIKeyScope = "default"
	// APIKeyScopeAdmin grants access to the admin endpoints (key management,
	// webhook management, audit log, retention).
	APIKeyScopeAdmin APIKeyScope = "admin"
)

// IsValidAPIKeyScope reports whether scope is a known value.
func IsValidAPIKeyScope(scope APIKeyScope) bool {
	switch scope {
	case APIKeyScopeDefault, APIKeyScopeAdmin:
		return true
	}
	return false
}

// APIKey describes a persisted bearer token used to authenticate HTTP requests.
//
// The raw token is only known to the caller at issue time; the application
// stores a SHA-256 hash and never persists the plaintext value.
type APIKey struct {
	ID         string
	Name       string
	Hash       string
	Scope      APIKeyScope
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// IssuedAPIKey is returned by Issue and carries the plaintext token that
// must be shown to the operator only once.
type IssuedAPIKey struct {
	APIKey APIKey
	Secret string
}
