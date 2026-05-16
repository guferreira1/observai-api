package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// ErrSetupAlreadyCompleted indicates the bootstrap admin endpoint was
// invoked after the initial admin user was already provisioned.
var ErrSetupAlreadyCompleted = errors.New("setup already completed")

// SetupState describes the operator-facing installation state.
type SetupState string

const (
	// SetupStatePending signals the operator must still create the initial
	// admin user before any other endpoint can be configured.
	SetupStatePending SetupState = "pending"
	// SetupStateDegraded signals the admin user exists but the installation
	// is missing at least one provider class (observability or LLM).
	SetupStateDegraded SetupState = "degraded"
	// SetupStateCompleted signals every prerequisite is in place.
	SetupStateCompleted SetupState = "completed"
)

// SetupStatus is the snapshot returned by Setup.Status.
type SetupStatus struct {
	AdminExists             bool
	LLMConfigured           bool
	ObservabilityConfigured bool
	SetupCompleted          bool
	State                   SetupState
}

// BootstrapAdminRequest carries the first-run admin creation payload.
type BootstrapAdminRequest struct {
	Name     string
	Email    string
	Password string
}

// Setup orchestrates the first-run installation flow.
type Setup struct {
	users     ports.UserRepository
	userAdmin *User
	inventory ports.ProviderInventory
	audit     *AuditLog
	now       func() time.Time
}

// NewSetup creates a Setup use case.
//
// userAdmin is reused to provision the initial admin so password hashing,
// validation and id generation match the regular admin creation flow.
// inventory may be nil in tests; when nil the readiness check assumes no
// providers are configured.
func NewSetup(users ports.UserRepository, userAdmin *User, inventory ports.ProviderInventory) *Setup {
	return &Setup{users: users, userAdmin: userAdmin, inventory: inventory, now: time.Now}
}

// WithAuditLog attaches an audit recorder so BootstrapAdmin can record the
// `setup.completed` event.
func (useCase *Setup) WithAuditLog(audit *AuditLog) *Setup {
	useCase.audit = audit
	return useCase
}

// Status reports whether the initial admin has been created and whether
// observability and LLM providers are wired.
func (useCase *Setup) Status(ctx context.Context) (SetupStatus, error) {
	count, err := useCase.users.Count(ctx)
	if err != nil {
		return SetupStatus{}, fmt.Errorf("count users: %w", err)
	}
	status := SetupStatus{AdminExists: count > 0}
	if useCase.inventory != nil {
		snapshot := useCase.inventory.Snapshot()
		status.ObservabilityConfigured = snapshot.ObservabilityProviders > 0
		status.LLMConfigured = snapshot.LLMProviders > 0
	}
	status.SetupCompleted = status.AdminExists && status.ObservabilityConfigured && status.LLMConfigured
	status.State = deriveSetupState(status)
	return status, nil
}

// BootstrapAdmin creates the initial admin user. The call is rejected with
// ErrSetupAlreadyCompleted when any user already exists so the public
// endpoint cannot be used to add admins after installation.
func (useCase *Setup) BootstrapAdmin(ctx context.Context, email, password string) (domain.User, error) {
	return useCase.BootstrapAdminWithOptions(ctx, BootstrapAdminRequest{
		Email:    email,
		Password: password,
	})
}

// BootstrapAdminWithOptions creates the initial admin user with profile data.
func (useCase *Setup) BootstrapAdminWithOptions(ctx context.Context, request BootstrapAdminRequest) (domain.User, error) {
	count, err := useCase.users.Count(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return domain.User{}, ErrSetupAlreadyCompleted
	}
	if useCase.userAdmin == nil {
		return domain.User{}, fmt.Errorf("setup: user use case not configured")
	}
	user, err := useCase.userAdmin.CreateWithOptions(ctx, UserCreateRequest{
		Name:     request.Name,
		Email:    request.Email,
		Password: request.Password,
		Role:     domain.RoleAdmin,
	})
	if err != nil {
		return domain.User{}, err
	}
	if useCase.audit != nil {
		_ = useCase.audit.Append(ctx, domain.AuditEntry{
			Actor:     user.Email,
			Method:    "POST",
			Path:      "/v1/setup/admin",
			Status:    201,
			CreatedAt: useCase.now().UTC(),
		})
	}
	return user, nil
}

func deriveSetupState(status SetupStatus) SetupState {
	switch {
	case !status.AdminExists:
		return SetupStatePending
	case status.SetupCompleted:
		return SetupStateCompleted
	default:
		return SetupStateDegraded
	}
}
