package usecase

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
)

type sequentialIDs struct {
	count int
}

func (gen *sequentialIDs) NextID(_ context.Context) (string, error) {
	gen.count++
	return seqID(gen.count), nil
}

func seqID(value int) string {
	return "id-" + intToString(value)
}

func intToString(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	negative := value < 0
	if negative {
		value = -value
	}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func newAuthFixture(t *testing.T) (*Auth, *inmemory.UserRepository, *inmemory.RefreshTokenRepository, *crypto.JWTSigner, *sequentialIDs) {
	t.Helper()
	users := inmemory.NewUserRepository()
	refresh := inmemory.NewRefreshTokenRepository()
	signer, err := crypto.NewJWTSigner(bytes.Repeat([]byte{0xab}, crypto.MinJWTSecretLength), "observai-api")
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	hasher := crypto.NewBcryptPasswordHasher(4)
	ids := &sequentialIDs{}
	auth := NewAuth(users, refresh, signer, hasher, ids, AuthOptions{AccessTokenTTL: 5 * time.Minute, RefreshTokenTTL: time.Hour})
	return auth, users, refresh, signer, ids
}

func seedAdminUser(t *testing.T, users *inmemory.UserRepository) domain.User {
	t.Helper()
	hash, err := crypto.HashPassword("CorrectHorse42", 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := domain.User{
		ID:           "user-1",
		Name:         "Admin User",
		Email:        "admin@observai.io",
		PasswordHash: hash,
		Role:         domain.RoleAdmin,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := users.Create(context.Background(), user); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return user
}

func TestAuthLoginIssuesAccessAndRefresh(t *testing.T) {
	auth, users, refresh, signer, _ := newAuthFixture(t)
	seedAdminUser(t, users)

	session, err := auth.Login(context.Background(), "admin@observai.io", "CorrectHorse42")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if session.Access.Value == "" || session.Refresh.Value == "" {
		t.Fatalf("expected non-empty tokens, got %+v", session)
	}

	claims, err := signer.Parse(session.Access.Value)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if claims.Subject != "user-1" || claims.Role != string(domain.RoleAdmin) {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	stored, err := refresh.FindByHash(context.Background(), HashRefreshToken(session.Refresh.Value))
	if err != nil {
		t.Fatalf("refresh stored: %v", err)
	}
	if stored.UserID != "user-1" || stored.FamilyID == "" {
		t.Fatalf("unexpected stored token: %+v", stored)
	}
}

func TestAuthLoginRejectsBadPassword(t *testing.T) {
	auth, users, _, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	_, err := auth.Login(context.Background(), "admin@observai.io", "wrong")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthLoginRejectsInactiveUser(t *testing.T) {
	auth, users, _, _, _ := newAuthFixture(t)
	user := seedAdminUser(t, users)
	if err := users.SetActive(context.Background(), user.ID, false, time.Now().UTC()); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	_, err := auth.Login(context.Background(), user.Email, "CorrectHorse42")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for inactive user, got %v", err)
	}
}

func TestAuthLoginRejectsUnknownUser(t *testing.T) {
	auth, _, _, _, _ := newAuthFixture(t)
	_, err := auth.Login(context.Background(), "ghost@observai.io", "whatever")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthRefreshRotatesTokenAndKeepsFamily(t *testing.T) {
	auth, users, refresh, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	first, err := auth.Login(context.Background(), "admin@observai.io", "CorrectHorse42")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	second, err := auth.Refresh(context.Background(), first.Refresh.Value)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.Refresh.Value == first.Refresh.Value {
		t.Fatalf("refresh secret was not rotated")
	}
	if second.Refresh.FamilyID != first.Refresh.FamilyID {
		t.Fatalf("family id changed: %s vs %s", first.Refresh.FamilyID, second.Refresh.FamilyID)
	}

	previous, err := refresh.FindByHash(context.Background(), HashRefreshToken(first.Refresh.Value))
	if err != nil {
		t.Fatalf("find previous: %v", err)
	}
	if previous.RevokedAt == nil {
		t.Fatalf("previous token should have been revoked")
	}
	if previous.ReplacedBy == nil || *previous.ReplacedBy != second.Refresh.ID {
		t.Fatalf("replaced_by chain broken: %+v", previous.ReplacedBy)
	}
}

func TestAuthRefreshReuseRevokesFamily(t *testing.T) {
	auth, users, refresh, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	first, _ := auth.Login(context.Background(), "admin@observai.io", "CorrectHorse42")
	second, _ := auth.Refresh(context.Background(), first.Refresh.Value)

	if _, err := auth.Refresh(context.Background(), first.Refresh.Value); !errors.Is(err, domain.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken on reuse, got %v", err)
	}

	latest, _ := refresh.FindByHash(context.Background(), HashRefreshToken(second.Refresh.Value))
	if latest.RevokedAt == nil {
		t.Fatalf("expected family revocation after reuse, got %+v", latest)
	}
}

func TestAuthRefreshRejectsExpiredToken(t *testing.T) {
	auth, users, _, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	auth.opts.RefreshTokenTTL = -time.Second
	session, _ := auth.Login(context.Background(), "admin@observai.io", "CorrectHorse42")
	_, err := auth.Refresh(context.Background(), session.Refresh.Value)
	if !errors.Is(err, domain.ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
	}
}

func TestAuthLogoutRevokesToken(t *testing.T) {
	auth, users, refresh, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	session, _ := auth.Login(context.Background(), "admin@observai.io", "CorrectHorse42")
	if err := auth.Logout(context.Background(), session.Refresh.Value); err != nil {
		t.Fatalf("logout: %v", err)
	}
	stored, _ := refresh.FindByHash(context.Background(), HashRefreshToken(session.Refresh.Value))
	if stored.RevokedAt == nil {
		t.Fatalf("token not revoked after logout")
	}
}

func TestAuthChangePasswordRotatesAndRevokesRefresh(t *testing.T) {
	auth, users, refresh, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	session, _ := auth.Login(context.Background(), "admin@observai.io", "CorrectHorse42")

	if err := auth.ChangePassword(context.Background(), "user-1", "CorrectHorse42", "NewPassw0rd!"); err != nil {
		t.Fatalf("change password: %v", err)
	}

	stored, _ := refresh.FindByHash(context.Background(), HashRefreshToken(session.Refresh.Value))
	if stored.RevokedAt == nil {
		t.Fatalf("expected previous refresh tokens revoked after password change")
	}
	if _, err := auth.Login(context.Background(), "admin@observai.io", "CorrectHorse42"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected old password to fail, got %v", err)
	}
	if _, err := auth.Login(context.Background(), "admin@observai.io", "NewPassw0rd!"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

func TestAuthUpdateProfileValidatesEmail(t *testing.T) {
	auth, users, _, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	_, err := auth.UpdateProfile(context.Background(), "user-1", "")
	if !errors.Is(err, domain.ErrInvalidUser) {
		t.Fatalf("expected ErrInvalidUser for empty email, got %v", err)
	}
}

func TestAuthUpdateProfileUpdatesName(t *testing.T) {
	auth, users, _, _, _ := newAuthFixture(t)
	seedAdminUser(t, users)
	user, err := auth.UpdateProfileWithOptions(context.Background(), "user-1", UserProfileUpdateRequest{
		Name:  "Gustavo Ferreira",
		Email: "admin@observai.io",
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if user.Name != "Gustavo Ferreira" {
		t.Fatalf("expected updated name, got %q", user.Name)
	}
}
