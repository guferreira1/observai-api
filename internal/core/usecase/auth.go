package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
)

const (
	refreshTokenSecretBytes = 32
	minPasswordLength       = 8
)

// AccessToken is the JWT access token returned to clients after a successful
// authentication.
type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}

// RefreshToken is the opaque refresh credential paired with an AccessToken.
type RefreshToken struct {
	ID        string
	Value     string
	ExpiresAt time.Time
	FamilyID  string
}

// AuthSession bundles the artifacts returned by login or refresh.
type AuthSession struct {
	User    domain.User
	Access  AccessToken
	Refresh RefreshToken
}

// AuthOptions tunes the lifetime of issued tokens.
type AuthOptions struct {
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// UserProfileUpdateRequest carries editable user profile fields.
type UserProfileUpdateRequest struct {
	Name  string
	Email string
}

// Auth orchestrates login, logout, refresh and profile management.
type Auth struct {
	users   ports.UserRepository
	refresh ports.RefreshTokenRepository
	signer  *crypto.JWTSigner
	ids     ports.IDGenerator
	opts    AuthOptions
	now     func() time.Time
}

// NewAuth creates an Auth use case.
//
// Zero-valued TTLs fall back to safe defaults: 15 minutes for access tokens
// and 7 days for refresh tokens.
func NewAuth(users ports.UserRepository, refresh ports.RefreshTokenRepository, signer *crypto.JWTSigner, ids ports.IDGenerator, opts AuthOptions) *Auth {
	if opts.AccessTokenTTL <= 0 {
		opts.AccessTokenTTL = 15 * time.Minute
	}
	if opts.RefreshTokenTTL <= 0 {
		opts.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	return &Auth{users: users, refresh: refresh, signer: signer, ids: ids, opts: opts, now: time.Now}
}

// Login validates the supplied credentials and issues a fresh session.
//
// Missing user, inactive user and bad password are deliberately collapsed
// into ErrInvalidCredentials so attackers cannot enumerate accounts.
func (useCase *Auth) Login(ctx context.Context, email, password string) (AuthSession, error) {
	user, err := useCase.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return AuthSession{}, domain.ErrInvalidCredentials
		}
		return AuthSession{}, fmt.Errorf("find user: %w", err)
	}
	if !user.IsActive {
		return AuthSession{}, domain.ErrInvalidCredentials
	}
	if err := crypto.VerifyPassword(user.PasswordHash, password); err != nil {
		return AuthSession{}, domain.ErrInvalidCredentials
	}
	now := useCase.now().UTC()
	_ = useCase.users.TouchLastLogin(ctx, user.ID, now)
	user.LastLoginAt = &now
	return useCase.issueSession(ctx, user, now, sessionRotation{})
}

// Refresh rotates the supplied refresh token. Reuse of a revoked token
// revokes the entire family to defeat replay-based session theft.
func (useCase *Auth) Refresh(ctx context.Context, refreshSecret string) (AuthSession, error) {
	hash := HashRefreshToken(refreshSecret)
	token, err := useCase.refresh.FindByHash(ctx, hash)
	if err != nil {
		return AuthSession{}, domain.ErrInvalidRefreshToken
	}
	now := useCase.now().UTC()
	if token.IsRevoked() {
		_ = useCase.refresh.RevokeFamily(ctx, token.FamilyID, now)
		return AuthSession{}, domain.ErrInvalidRefreshToken
	}
	if token.IsExpired(now) {
		return AuthSession{}, domain.ErrInvalidRefreshToken
	}
	user, err := useCase.users.FindByID(ctx, token.UserID)
	if err != nil || !user.IsActive {
		return AuthSession{}, domain.ErrInvalidRefreshToken
	}
	return useCase.issueSession(ctx, user, now, sessionRotation{FamilyID: token.FamilyID, ReplaceID: token.ID})
}

// StartSessionForUser issues an authenticated session for a known active user.
func (useCase *Auth) StartSessionForUser(ctx context.Context, user domain.User) (AuthSession, error) {
	if !user.IsActive {
		return AuthSession{}, domain.ErrInvalidCredentials
	}
	now := useCase.now().UTC()
	_ = useCase.users.TouchLastLogin(ctx, user.ID, now)
	user.LastLoginAt = &now
	return useCase.issueSession(ctx, user, now, sessionRotation{})
}

// Logout revokes the supplied refresh token. Other tokens in the family
// remain valid so a user signed in on multiple devices keeps the rest of
// their sessions intact.
func (useCase *Auth) Logout(ctx context.Context, refreshSecret string) error {
	hash := HashRefreshToken(refreshSecret)
	token, err := useCase.refresh.FindByHash(ctx, hash)
	if err != nil {
		return nil
	}
	if token.IsRevoked() {
		return nil
	}
	return useCase.refresh.Revoke(ctx, token.ID, nil, useCase.now().UTC())
}

// Me returns the user matching the supplied identifier.
func (useCase *Auth) Me(ctx context.Context, userID string) (domain.User, error) {
	return useCase.users.FindByID(ctx, userID)
}

// UpdateProfile updates the email of the supplied user.
func (useCase *Auth) UpdateProfile(ctx context.Context, userID string, email string) (domain.User, error) {
	return useCase.UpdateProfileWithOptions(ctx, userID, UserProfileUpdateRequest{Email: email})
}

// UpdateProfileWithOptions updates editable profile fields for the supplied user.
func (useCase *Auth) UpdateProfileWithOptions(ctx context.Context, userID string, request UserProfileUpdateRequest) (domain.User, error) {
	cleanedEmail, err := normalizeEmail(request.Email)
	if err != nil {
		return domain.User{}, err
	}
	user, err := useCase.users.FindByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	cleanedName := strings.TrimSpace(request.Name)
	if cleanedName == "" {
		cleanedName = normalizeUserName(user.Name, cleanedEmail)
	}
	now := useCase.now().UTC()
	if err := useCase.users.UpdateProfile(ctx, userID, cleanedName, cleanedEmail, now); err != nil {
		return domain.User{}, err
	}
	return useCase.users.FindByID(ctx, userID)
}

// UpdatePreferences stores user-owned UI preferences for the supplied user.
func (useCase *Auth) UpdatePreferences(ctx context.Context, userID string, preferences domain.UserPreferences) (domain.User, error) {
	normalized := domain.NormalizeUserPreferences(preferences)
	now := useCase.now().UTC()
	if err := useCase.users.UpdatePreferences(ctx, userID, normalized, now); err != nil {
		return domain.User{}, err
	}
	return useCase.users.FindByID(ctx, userID)
}

// ChangePassword verifies the current password, stores a fresh bcrypt hash
// and revokes every active refresh token belonging to the user so a stolen
// session cannot survive the rotation.
func (useCase *Auth) ChangePassword(ctx context.Context, userID, current, replacement string) error {
	if len(replacement) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", domain.ErrInvalidUser, minPasswordLength)
	}
	user, err := useCase.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := crypto.VerifyPassword(user.PasswordHash, current); err != nil {
		return domain.ErrInvalidCredentials
	}
	hash, err := crypto.HashPassword(replacement, 0)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	now := useCase.now().UTC()
	if err := useCase.users.UpdatePassword(ctx, userID, hash, now); err != nil {
		return err
	}
	_ = useCase.users.SetMustChangePassword(ctx, userID, false, now)
	_ = useCase.refresh.RevokeAllForUser(ctx, userID, now)
	return nil
}

type sessionRotation struct {
	FamilyID  string
	ReplaceID string
}

func (useCase *Auth) issueSession(ctx context.Context, user domain.User, now time.Time, rotation sessionRotation) (AuthSession, error) {
	accessJTI, err := useCase.ids.NextID(ctx)
	if err != nil {
		return AuthSession{}, fmt.Errorf("generate access jti: %w", err)
	}
	accessClaims := crypto.JWTClaims{
		Subject:   user.ID,
		Role:      string(user.Role),
		JTI:       accessJTI,
		IssuedAt:  now,
		ExpiresAt: now.Add(useCase.opts.AccessTokenTTL),
	}
	accessValue, err := useCase.signer.Sign(accessClaims)
	if err != nil {
		return AuthSession{}, fmt.Errorf("sign access token: %w", err)
	}
	refreshID, err := useCase.ids.NextID(ctx)
	if err != nil {
		return AuthSession{}, fmt.Errorf("generate refresh id: %w", err)
	}
	familyID := rotation.FamilyID
	if familyID == "" {
		familyID, err = useCase.ids.NextID(ctx)
		if err != nil {
			return AuthSession{}, fmt.Errorf("generate refresh family id: %w", err)
		}
	}
	refreshSecret, err := generateRefreshSecret()
	if err != nil {
		return AuthSession{}, err
	}
	refreshExpiry := now.Add(useCase.opts.RefreshTokenTTL)
	persisted := domain.RefreshToken{
		ID:        refreshID,
		UserID:    user.ID,
		TokenHash: HashRefreshToken(refreshSecret),
		FamilyID:  familyID,
		ExpiresAt: refreshExpiry,
		CreatedAt: now,
	}
	if err := useCase.refresh.Create(ctx, persisted); err != nil {
		return AuthSession{}, fmt.Errorf("persist refresh token: %w", err)
	}
	if rotation.ReplaceID != "" {
		if err := useCase.refresh.Revoke(ctx, rotation.ReplaceID, &refreshID, now); err != nil {
			return AuthSession{}, fmt.Errorf("revoke previous refresh token: %w", err)
		}
	}
	return AuthSession{
		User: user,
		Access: AccessToken{
			Value:     accessValue,
			ExpiresAt: accessClaims.ExpiresAt,
		},
		Refresh: RefreshToken{
			ID:        refreshID,
			Value:     refreshSecret,
			ExpiresAt: refreshExpiry,
			FamilyID:  familyID,
		},
	}, nil
}

// HashRefreshToken returns the canonical hex-encoded SHA-256 digest used to
// persist refresh tokens.
func HashRefreshToken(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func generateRefreshSecret() (string, error) {
	buffer := make([]byte, refreshTokenSecretBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate refresh secret: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func normalizeEmail(email string) (string, error) {
	cleaned := strings.ToLower(strings.TrimSpace(email))
	if cleaned == "" {
		return "", fmt.Errorf("%w: email is required", domain.ErrInvalidUser)
	}
	if _, err := mail.ParseAddress(cleaned); err != nil {
		return "", fmt.Errorf("%w: email is not valid", domain.ErrInvalidUser)
	}
	return cleaned, nil
}
