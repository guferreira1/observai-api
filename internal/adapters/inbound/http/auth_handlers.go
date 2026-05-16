package http

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
	"github.com/guferreira1/observai-api/internal/core/usecase"
)

// CookieConfig controls the attributes attached to authentication cookies.
type CookieConfig struct {
	Domain   string
	Secure   bool
	SameSite stdhttp.SameSite
}

func (config CookieConfig) sameSite() stdhttp.SameSite {
	if config.SameSite == 0 {
		return stdhttp.SameSiteLaxMode
	}
	return config.SameSite
}

// LoginRequestDto is the payload accepted by POST /v1/auth/login.
type LoginRequestDto struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// UpdateProfileRequestDto is the payload accepted by PATCH /v1/me.
type UpdateProfileRequestDto struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email" validate:"required,email"`
}

// UpdatePreferencesRequestDto is the payload accepted by PATCH /v1/me/preferences.
type UpdatePreferencesRequestDto struct {
	Locale    *string `json:"locale,omitempty"`
	Timezone  *string `json:"timezone,omitempty"`
	Theme     *string `json:"theme,omitempty" validate:"omitempty,oneof=light dark system"`
	DenseMode *bool   `json:"denseMode,omitempty"`
}

// ChangePasswordRequestDto is the payload accepted by POST /v1/me/password.
type ChangePasswordRequestDto struct {
	CurrentPassword string `json:"currentPassword" validate:"required"`
	NewPassword     string `json:"newPassword" validate:"required,min=8"`
}

// CreateUserRequestDto is the payload accepted by POST /v1/admin/users.
type CreateUserRequestDto struct {
	Name               string `json:"name,omitempty"`
	Email              string `json:"email" validate:"required,email"`
	Password           string `json:"password" validate:"required,min=8"`
	Role               string `json:"role" validate:"required,oneof=admin operator viewer"`
	MustChangePassword bool   `json:"mustChangePassword,omitempty"`
}

// UpdateUserRequestDto is the payload accepted by PATCH /v1/admin/users/{id}.
type UpdateUserRequestDto struct {
	Role               *string `json:"role,omitempty" validate:"omitempty,oneof=admin operator viewer"`
	IsActive           *bool   `json:"isActive,omitempty"`
	MustChangePassword *bool   `json:"mustChangePassword,omitempty"`
}

// UserResponseDto is the public projection of a domain.User.
type UserResponseDto struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Email              string         `json:"email"`
	Role               string         `json:"role"`
	IsActive           bool           `json:"isActive"`
	MustChangePassword bool           `json:"mustChangePassword"`
	Preferences        PreferencesDto `json:"preferences"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
	LastLoginAt        *string        `json:"lastLoginAt,omitempty"`
}

// PreferencesDto is the frontend-safe projection of user preferences.
type PreferencesDto struct {
	Locale    string `json:"locale"`
	Timezone  string `json:"timezone"`
	Theme     string `json:"theme"`
	DenseMode bool   `json:"denseMode"`
}

// MySessionsResponseDto lists sessions visible to the current user.
type MySessionsResponseDto struct {
	Items []MySessionDto `json:"items"`
}

// MySessionDto describes the current access-token-backed browser session.
type MySessionDto struct {
	ID        string `json:"id"`
	Current   bool   `json:"current"`
	IssuedAt  string `json:"issuedAt,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// MyKeysResponseDto lists API keys visible from the current credential.
type MyKeysResponseDto struct {
	Items []APIKeyDto `json:"items"`
}

// SessionResponseDto is returned by POST /v1/auth/login and /v1/auth/refresh.
//
// Cookies carry the actual credentials; the body returns the user profile
// and the CSRF token so the SPA can attach the X-CSRF-Token header on
// subsequent mutations.
type SessionResponseDto struct {
	User      UserResponseDto `json:"user"`
	CSRFToken string          `json:"csrfToken"`
	ExpiresAt string          `json:"expiresAt"`
}

func (router *Router) toUserResponseDto(user domain.User) UserResponseDto {
	dto := UserResponseDto{
		ID:                 user.ID,
		Name:               userResponseName(user),
		Email:              user.Email,
		Role:               string(user.Role),
		IsActive:           user.IsActive,
		MustChangePassword: user.MustChangePassword,
		Preferences:        toPreferencesDto(user.Preferences),
		CreatedAt:          router.formatTime(user.CreatedAt),
		UpdatedAt:          router.formatTime(user.UpdatedAt),
	}
	if user.LastLoginAt != nil {
		last := router.formatTime(*user.LastLoginAt)
		dto.LastLoginAt = &last
	}
	return dto
}

func userResponseName(user domain.User) string {
	name := strings.TrimSpace(user.Name)
	if name != "" {
		return name
	}
	return user.Email
}

func toPreferencesDto(preferences domain.UserPreferences) PreferencesDto {
	normalized := domain.NormalizeUserPreferences(preferences)
	return PreferencesDto{
		Locale:    normalized.Locale,
		Timezone:  normalized.Timezone,
		Theme:     normalized.Theme,
		DenseMode: normalized.DenseMode,
	}
}

func toDomainPreferences(dto PreferencesDto) domain.UserPreferences {
	return domain.UserPreferences{
		Locale:    dto.Locale,
		Timezone:  dto.Timezone,
		Theme:     dto.Theme,
		DenseMode: dto.DenseMode,
	}
}

func (router *Router) handleLogin(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto LoginRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	session, err := router.sessions.Login(request.Context(), dto.Email, dto.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			AnnotateAudit(request, AuditAnnotation{
				Action:       "auth.login_failed",
				ResourceType: "user",
				Metadata:     map[string]string{"email": strings.ToLower(strings.TrimSpace(dto.Email))},
			})
			router.writeError(writer, requestID, startedAt, stdhttp.StatusUnauthorized, "invalid_credentials", "email or password is invalid")
			return
		}
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	AnnotateAudit(request, AuditAnnotation{
		Action:       "auth.login",
		ResourceType: "user",
		ResourceID:   session.User.ID,
		Metadata:     map[string]string{"email": session.User.Email},
	})

	router.writeSessionResponse(writer, requestID, startedAt, stdhttp.StatusOK, session)
}

func (router *Router) handleLogout(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	if cookie, err := request.Cookie(RefreshCookieName); err == nil && cookie.Value != "" {
		_ = router.sessions.Logout(request.Context(), cookie.Value)
	}
	router.clearSessionCookies(writer)
	AnnotateAudit(request, AuditAnnotation{Action: "auth.logout", ResourceType: "user"})
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

func (router *Router) handleRefresh(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	cookie, err := request.Cookie(RefreshCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusUnauthorized, "invalid_refresh_token", "refresh cookie missing or invalid")
		return
	}

	session, err := router.sessions.Refresh(request.Context(), cookie.Value)
	if err != nil {
		router.clearSessionCookies(writer)
		router.writeError(writer, requestID, startedAt, stdhttp.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid or expired")
		return
	}

	router.writeSessionResponse(writer, requestID, startedAt, stdhttp.StatusOK, session)
}

func (router *Router) handleMe(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	principal, ok := PrincipalFromContext(request.Context())
	if !ok || principal.Source != AuthSourceUser {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusForbidden, "forbidden", "user session required")
		return
	}
	user, err := router.sessions.Me(request.Context(), principal.UserID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, router.toUserResponseDto(user))
}

func (router *Router) handleUpdateMe(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	principal, ok := PrincipalFromContext(request.Context())
	if !ok || principal.Source != AuthSourceUser {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusForbidden, "forbidden", "user session required")
		return
	}

	var dto UpdateProfileRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	user, err := router.sessions.UpdateProfileWithOptions(request.Context(), principal.UserID, usecase.UserProfileUpdateRequest{
		Name:  dto.Name,
		Email: dto.Email,
	})
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, router.toUserResponseDto(user))
}

func (router *Router) handleChangePassword(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	principal, ok := PrincipalFromContext(request.Context())
	if !ok || principal.Source != AuthSourceUser {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusForbidden, "forbidden", "user session required")
		return
	}

	var dto ChangePasswordRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	if err := router.sessions.ChangePassword(request.Context(), principal.UserID, dto.CurrentPassword, dto.NewPassword); err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			router.writeError(writer, requestID, startedAt, stdhttp.StatusUnauthorized, "invalid_credentials", "current password is incorrect")
			return
		}
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.clearSessionCookies(writer)
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

func (router *Router) handleGetPreferences(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	principal, ok := PrincipalFromContext(request.Context())
	if !ok || principal.Source != AuthSourceUser {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusForbidden, "forbidden", "user session required")
		return
	}
	user, err := router.sessions.Me(request.Context(), principal.UserID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toPreferencesDto(user.Preferences))
}

func (router *Router) handleUpdatePreferences(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	principal, ok := PrincipalFromContext(request.Context())
	if !ok || principal.Source != AuthSourceUser {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusForbidden, "forbidden", "user session required")
		return
	}

	var dto UpdatePreferencesRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	current, err := router.sessions.Me(request.Context(), principal.UserID)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	updated := mergePreferences(toPreferencesDto(current.Preferences), dto)
	user, err := router.sessions.UpdatePreferences(request.Context(), principal.UserID, toDomainPreferences(updated))
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toPreferencesDto(user.Preferences))
}

func mergePreferences(current PreferencesDto, update UpdatePreferencesRequestDto) PreferencesDto {
	applyOptionalString(&current.Locale, update.Locale)
	applyOptionalString(&current.Timezone, update.Timezone)
	applyOptionalString(&current.Theme, update.Theme)
	applyOptionalBool(&current.DenseMode, update.DenseMode)
	return toPreferencesDto(toDomainPreferences(current))
}

func applyOptionalString(target *string, value *string) {
	if value != nil {
		*target = *value
	}
}

func applyOptionalBool(target *bool, value *bool) {
	if value != nil {
		*target = *value
	}
}

func (router *Router) handleListSessions(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	principal, ok := PrincipalFromContext(request.Context())
	if !ok || principal.Source != AuthSourceUser {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusForbidden, "forbidden", "user session required")
		return
	}
	session := MySessionDto{
		ID:      principal.SessionID,
		Current: true,
	}
	if !principal.SessionIssuedAt.IsZero() {
		session.IssuedAt = principal.SessionIssuedAt.Format(time.RFC3339)
	}
	if !principal.SessionExpiresAt.IsZero() {
		session.ExpiresAt = principal.SessionExpiresAt.Format(time.RFC3339)
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, MySessionsResponseDto{Items: []MySessionDto{session}})
}

func (router *Router) handleListMyKeys(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	principal, ok := PrincipalFromContext(request.Context())
	if !ok {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusForbidden, "forbidden", "authenticated principal required")
		return
	}
	if principal.Source != AuthSourceAPIKey || principal.KeyID == "" {
		router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, MyKeysResponseDto{Items: []APIKeyDto{}})
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, MyKeysResponseDto{Items: []APIKeyDto{{
		ID:     principal.KeyID,
		Name:   principal.Name,
		Scopes: scopesToStrings(principal.Scopes),
		Masked: policy.MaskSecret("oai_" + principal.KeyID),
	}}})
}

func (router *Router) handleListUsers(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	limit, offset := paginationFromQuery(request)
	users, err := router.users.List(request.Context(), limit, offset)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	items := make([]UserResponseDto, 0, len(users))
	for _, user := range users {
		items = append(items, router.toUserResponseDto(user))
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, items)
}

func (router *Router) handleCreateUser(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto CreateUserRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	user, err := router.users.CreateWithOptions(request.Context(), usecase.UserCreateRequest{
		Name:               dto.Name,
		Email:              dto.Email,
		Password:           dto.Password,
		Role:               domain.Role(dto.Role),
		MustChangePassword: dto.MustChangePassword,
	})
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	AnnotateAudit(request, AuditAnnotation{
		Action:       "user.created",
		ResourceType: "user",
		ResourceID:   user.ID,
		Metadata:     map[string]string{"email": user.Email, "role": string(user.Role)},
	})
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, router.toUserResponseDto(user))
}

func (router *Router) handleGetUser(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	user, err := router.users.Get(request.Context(), chi.URLParam(request, "userID"))
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, router.toUserResponseDto(user))
}

func (router *Router) handleUpdateUser(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	id := chi.URLParam(request, "userID")
	var dto UpdateUserRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	updated, err := router.applyUserUpdate(request, id, dto)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	AnnotateAudit(request, AuditAnnotation{
		Action:       "user.updated",
		ResourceType: "user",
		ResourceID:   updated.ID,
		Metadata:     map[string]string{"role": string(updated.Role), "isActive": formatBool(updated.IsActive)},
	})
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, router.toUserResponseDto(updated))
}

func (router *Router) applyUserUpdate(request *stdhttp.Request, id string, dto UpdateUserRequestDto) (domain.User, error) {
	current, err := router.users.Get(request.Context(), id)
	if err != nil {
		return domain.User{}, err
	}
	if dto.Role != nil && *dto.Role != string(current.Role) {
		current, err = router.users.UpdateRole(request.Context(), id, domain.Role(*dto.Role))
		if err != nil {
			return domain.User{}, err
		}
	}
	if dto.IsActive != nil && *dto.IsActive != current.IsActive {
		current, err = router.users.SetActive(request.Context(), id, *dto.IsActive)
		if err != nil {
			return domain.User{}, err
		}
	}
	if dto.MustChangePassword != nil && *dto.MustChangePassword != current.MustChangePassword {
		current, err = router.users.SetMustChangePassword(request.Context(), id, *dto.MustChangePassword)
		if err != nil {
			return domain.User{}, err
		}
	}
	return current, nil
}

func (router *Router) handleDeleteUser(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	userID := chi.URLParam(request, "userID")
	if err := router.users.Delete(request.Context(), userID); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	AnnotateAudit(request, AuditAnnotation{Action: "user.deleted", ResourceType: "user", ResourceID: userID})
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusNoContent, struct{}{})
}

func (router *Router) setSessionCookies(writer stdhttp.ResponseWriter, session usecase.AuthSession, csrf string) {
	cookieConfig := router.options.Cookies
	stdhttp.SetCookie(writer, &stdhttp.Cookie{
		Name:     SessionCookieName,
		Value:    session.Access.Value,
		Path:     "/",
		Domain:   cookieConfig.Domain,
		Expires:  session.Access.ExpiresAt,
		MaxAge:   int(time.Until(session.Access.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   cookieConfig.Secure,
		SameSite: cookieConfig.sameSite(),
	})
	stdhttp.SetCookie(writer, &stdhttp.Cookie{
		Name:     RefreshCookieName,
		Value:    session.Refresh.Value,
		Path:     "/v1/auth",
		Domain:   cookieConfig.Domain,
		Expires:  session.Refresh.ExpiresAt,
		MaxAge:   int(time.Until(session.Refresh.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   cookieConfig.Secure,
		SameSite: cookieConfig.sameSite(),
	})
	stdhttp.SetCookie(writer, &stdhttp.Cookie{
		Name:     CSRFCookieName,
		Value:    csrf,
		Path:     "/",
		Domain:   cookieConfig.Domain,
		Expires:  session.Access.ExpiresAt,
		MaxAge:   int(time.Until(session.Access.ExpiresAt).Seconds()),
		HttpOnly: false,
		Secure:   cookieConfig.Secure,
		SameSite: cookieConfig.sameSite(),
	})
}

func (router *Router) writeSessionResponse(writer stdhttp.ResponseWriter, requestID string, startedAt time.Time, status int, session usecase.AuthSession) {
	csrf, err := generateCSRFToken()
	if err != nil {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusInternalServerError, "internal_error", "could not generate csrf token")
		return
	}
	router.setSessionCookies(writer, session, csrf)
	router.writeSuccess(writer, requestID, startedAt, status, SessionResponseDto{
		User:      router.toUserResponseDto(session.User),
		CSRFToken: csrf,
		ExpiresAt: router.formatTime(session.Access.ExpiresAt),
	})
}

func (router *Router) clearSessionCookies(writer stdhttp.ResponseWriter) {
	cookieConfig := router.options.Cookies
	for _, name := range []string{SessionCookieName, RefreshCookieName, CSRFCookieName} {
		path := "/"
		if name == RefreshCookieName {
			path = "/v1/auth"
		}
		stdhttp.SetCookie(writer, &stdhttp.Cookie{
			Name:     name,
			Value:    "",
			Path:     path,
			Domain:   cookieConfig.Domain,
			MaxAge:   -1,
			HttpOnly: name != CSRFCookieName,
			Secure:   cookieConfig.Secure,
			SameSite: cookieConfig.sameSite(),
		})
	}
}

func generateCSRFToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
