package http

import (
	"errors"
	stdhttp "net/http"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/usecase"
)

// SetupStatusResponseDto is the payload returned by GET /v1/setup/status.
type SetupStatusResponseDto struct {
	AdminExists             bool   `json:"adminExists"`
	LLMConfigured           bool   `json:"llmConfigured"`
	ObservabilityConfigured bool   `json:"observabilityConfigured"`
	SetupCompleted          bool   `json:"setupCompleted"`
	State                   string `json:"state"`
}

// BootstrapAdminRequestDto is the payload accepted by POST /v1/setup/admin.
type BootstrapAdminRequestDto struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

func toSetupStatusResponseDto(status usecase.SetupStatus) SetupStatusResponseDto {
	return SetupStatusResponseDto{
		AdminExists:             status.AdminExists,
		LLMConfigured:           status.LLMConfigured,
		ObservabilityConfigured: status.ObservabilityConfigured,
		SetupCompleted:          status.SetupCompleted,
		State:                   string(status.State),
	}
}

func (router *Router) handleSetupStatus(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	status, err := router.setup.Status(request.Context())
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusOK, toSetupStatusResponseDto(status))
}

func (router *Router) handleBootstrapAdmin(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	startedAt := time.Now()
	requestID := router.requestID(request)

	var dto BootstrapAdminRequestDto
	if err := decodeRequestBody(request, &dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if err := router.validate.Struct(dto); err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}

	user, err := router.setup.BootstrapAdminWithOptions(request.Context(), usecase.BootstrapAdminRequest{
		Name:     dto.Name,
		Email:    dto.Email,
		Password: dto.Password,
	})
	defer func() {
		if user.ID != "" {
			AnnotateAudit(request, AuditAnnotation{
				Action:       "setup.completed",
				ResourceType: "user",
				ResourceID:   user.ID,
				Metadata:     map[string]string{"email": user.Email},
			})
		}
	}()
	if err != nil {
		if errors.Is(err, usecase.ErrSetupAlreadyCompleted) {
			router.writeError(writer, requestID, startedAt, stdhttp.StatusConflict, "setup_already_completed", "initial admin already provisioned")
			return
		}
		if errors.Is(err, domain.ErrInvalidUser) {
			router.writeDomainError(writer, requestID, startedAt, err)
			return
		}
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			router.writeError(writer, requestID, startedAt, stdhttp.StatusConflict, "user_already_exists", "email is already registered")
			return
		}
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	if router.sessions == nil {
		router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, router.toUserResponseDto(user))
		return
	}
	session, err := router.sessions.StartSessionForUser(request.Context(), user)
	if err != nil {
		router.writeDomainError(writer, requestID, startedAt, err)
		return
	}
	csrf, err := generateCSRFToken()
	if err != nil {
		router.writeError(writer, requestID, startedAt, stdhttp.StatusInternalServerError, "internal_error", "could not generate csrf token")
		return
	}
	router.setSessionCookies(writer, session, csrf)
	router.writeSuccess(writer, requestID, startedAt, stdhttp.StatusCreated, SessionResponseDto{
		User:      router.toUserResponseDto(session.User),
		CSRFToken: csrf,
		ExpiresAt: router.formatTime(session.Access.ExpiresAt),
	})
}
