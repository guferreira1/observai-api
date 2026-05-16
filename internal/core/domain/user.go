package domain

import (
	"errors"
	"strings"
	"time"
)

// ErrUserNotFound is returned when a referenced user does not exist or has
// been deactivated.
var ErrUserNotFound = errors.New("user not found")

// ErrInvalidCredentials is returned when an email/password combination does
// not match a stored user. Callers must surface a generic message to avoid
// account-enumeration oracles.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidUser is returned when user input fails validation.
var ErrInvalidUser = errors.New("invalid user")

// ErrUserAlreadyExists is returned when creating a user whose email is
// already registered.
var ErrUserAlreadyExists = errors.New("user already exists")

// ErrInvalidRefreshToken is returned when a refresh-token payload cannot be
// resolved, is expired or has already been used (reuse attack).
var ErrInvalidRefreshToken = errors.New("invalid refresh token")

// Role names the authorization level granted to a user.
type Role string

const (
	// RoleAdmin grants full access including admin endpoints.
	RoleAdmin Role = "admin"
	// RoleOperator can submit analyses and use the chat.
	RoleOperator Role = "operator"
	// RoleViewer can only read analyses and chat history.
	RoleViewer Role = "viewer"
)

// IsValidRole reports whether the supplied role is a known value.
func IsValidRole(role Role) bool {
	switch role {
	case RoleAdmin, RoleOperator, RoleViewer:
		return true
	}
	return false
}

// User describes an authenticated principal stored in the application
// database.
//
// PasswordHash is never returned to clients. The HTTP layer projects User
// onto a transport DTO that omits sensitive fields.
type User struct {
	ID                 string
	Name               string
	Email              string
	PasswordHash       string
	Role               Role
	IsActive           bool
	MustChangePassword bool
	Preferences        UserPreferences
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        *time.Time
}

// UserPreferences stores user-owned UI preferences that are safe to expose
// through /v1/me/preferences.
type UserPreferences struct {
	Locale    string `json:"locale"`
	Timezone  string `json:"timezone"`
	Theme     string `json:"theme"`
	DenseMode bool   `json:"denseMode"`
}

// NormalizeUserPreferences applies stable defaults and drops unsupported values.
func NormalizeUserPreferences(preferences UserPreferences) UserPreferences {
	normalized := UserPreferences{
		Locale:    normalizePreferenceValue(preferences.Locale, "en-US"),
		Timezone:  normalizePreferenceValue(preferences.Timezone, "UTC"),
		Theme:     normalizeTheme(preferences.Theme),
		DenseMode: preferences.DenseMode,
	}
	return normalized
}

func normalizePreferenceValue(value string, fallback string) string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

func normalizeTheme(value string) string {
	switch strings.TrimSpace(value) {
	case "light", "dark", "system":
		return value
	default:
		return "system"
	}
}

// RefreshToken describes a persisted refresh credential.
//
// TokenHash is the SHA-256 hash of the opaque value held by the client.
// FamilyID groups tokens issued through rotation: detecting a reused
// revoked token in a family revokes the entire family to defeat replay.
type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  string
	FamilyID   string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
	CreatedAt  time.Time
}

// IsRevoked reports whether the token has been revoked.
func (token RefreshToken) IsRevoked() bool { return token.RevokedAt != nil }

// IsExpired reports whether the token expiry is in the past compared to now.
func (token RefreshToken) IsExpired(now time.Time) bool { return !token.ExpiresAt.After(now) }
