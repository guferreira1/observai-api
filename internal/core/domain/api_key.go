package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// ErrAPIKeyNotFound is returned when a referenced API key does not exist.
var ErrAPIKeyNotFound = errors.New("api key not found")

// ErrAPIKeyRevoked is returned when a referenced API key has been revoked.
var ErrAPIKeyRevoked = errors.New("api key revoked")

// ErrAPIKeyExpired is returned when a referenced API key is past its
// expiration timestamp.
var ErrAPIKeyExpired = errors.New("api key expired")

// ErrInvalidAPIKey is returned when API key input fails validation.
var ErrInvalidAPIKey = errors.New("invalid api key")

// APIKeyScope is a fine-grained authorization scope embedded in an API key.
//
// Scopes follow the "<resource>:<action>" convention so role-based guards
// can match them prefix-wise (e.g. anything with "admin:*" is admin-class).
type APIKeyScope string

const (
	// APIKeyScopeAnalysisRead grants read access to analyses and chat history.
	APIKeyScopeAnalysisRead APIKeyScope = "analysis:read"
	// APIKeyScopeAnalysisWrite grants permission to submit and cancel analyses.
	APIKeyScopeAnalysisWrite APIKeyScope = "analysis:write"
	// APIKeyScopeChatWrite grants permission to interact with the analysis chat.
	APIKeyScopeChatWrite APIKeyScope = "chat:write"
	// APIKeyScopeAdminRead grants read access to administrative endpoints.
	APIKeyScopeAdminRead APIKeyScope = "admin:read"
	// APIKeyScopeAdminWrite grants mutation access to administrative endpoints.
	APIKeyScopeAdminWrite APIKeyScope = "admin:write"
)

// AllAPIKeyScopes lists every supported scope value.
//
// Used to validate operator-supplied scopes and to populate the "admin"
// scope set granted to static admin keys for backwards compatibility.
func AllAPIKeyScopes() []APIKeyScope {
	return []APIKeyScope{
		APIKeyScopeAnalysisRead,
		APIKeyScopeAnalysisWrite,
		APIKeyScopeChatWrite,
		APIKeyScopeAdminRead,
		APIKeyScopeAdminWrite,
	}
}

// IsValidAPIKeyScope reports whether scope is a known value.
func IsValidAPIKeyScope(scope APIKeyScope) bool {
	switch scope {
	case APIKeyScopeAnalysisRead,
		APIKeyScopeAnalysisWrite,
		APIKeyScopeChatWrite,
		APIKeyScopeAdminRead,
		APIKeyScopeAdminWrite:
		return true
	}
	return false
}

// NormalizeAPIKeyScopes trims, lower-cases, de-duplicates and sorts the
// supplied scope list. Unknown values are rejected with ErrInvalidAPIKey.
func NormalizeAPIKeyScopes(scopes []APIKeyScope) ([]APIKeyScope, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	seen := make(map[APIKeyScope]struct{}, len(scopes))
	out := make([]APIKeyScope, 0, len(scopes))
	for _, raw := range scopes {
		scope := APIKeyScope(strings.ToLower(strings.TrimSpace(string(raw))))
		if scope == "" {
			continue
		}
		if !IsValidAPIKeyScope(scope) {
			return nil, ErrInvalidAPIKey
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// APIKey describes a persisted bearer token used to authenticate HTTP requests.
//
// The raw token is only known to the caller at issue time; the application
// stores a SHA-256 hash and never persists the plaintext value. Scopes
// drives authorization; Description is a free-form operator hint.
type APIKey struct {
	ID          string
	Name        string
	Description string
	Hash        string
	Scopes      []APIKeyScope
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

// IsRevoked reports whether the key has been revoked.
func (key APIKey) IsRevoked() bool { return key.RevokedAt != nil }

// IsExpired reports whether the key is past its expiration timestamp.
func (key APIKey) IsExpired(now time.Time) bool {
	return key.ExpiresAt != nil && !key.ExpiresAt.After(now)
}

// HasScope reports whether the key carries the supplied scope.
func (key APIKey) HasScope(target APIKeyScope) bool {
	for _, scope := range key.Scopes {
		if scope == target {
			return true
		}
	}
	return false
}

// IssuedAPIKey is returned by Issue and carries the plaintext token that
// must be shown to the operator only once.
type IssuedAPIKey struct {
	APIKey APIKey
	Secret string
}
