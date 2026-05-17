package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/guferreira1/observai-api/internal/adapters/outbound/inmemory"
	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/ports"
	"github.com/guferreira1/observai-api/internal/platform/crypto"
)

type stubInventory struct {
	observability int
	llm           int
}

func (stub stubInventory) Snapshot() ports.ProviderInventorySnapshot {
	return ports.ProviderInventorySnapshot{
		ObservabilityProviders: stub.observability,
		LLMProviders:           stub.llm,
	}
}

func newSetupFixture(t *testing.T, inventory ports.ProviderInventory) (*Setup, *inmemory.UserRepository) {
	t.Helper()
	users := inmemory.NewUserRepository()
	refresh := inmemory.NewRefreshTokenRepository()
	ids := &sequentialIDs{}
	userAdmin := NewUser(users, refresh, crypto.NewBcryptPasswordHasher(4), ids)
	return NewSetup(users, userAdmin, inventory), users
}

func TestSetupStatusReportsPendingWhenNoUsers(t *testing.T) {
	setup, _ := newSetupFixture(t, stubInventory{observability: 1, llm: 1})
	status, err := setup.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != SetupStatePending || status.AdminExists || status.SetupCompleted {
		t.Fatalf("expected pending state, got %+v", status)
	}
}

func TestSetupStatusReportsDegradedWhenMissingProviders(t *testing.T) {
	setup, _ := newSetupFixture(t, stubInventory{observability: 0, llm: 1})
	_, err := setup.BootstrapAdmin(context.Background(), "admin@observai.io", "CorrectHorse42")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	status, err := setup.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != SetupStateDegraded {
		t.Fatalf("expected degraded, got %s (%+v)", status.State, status)
	}
	if !status.AdminExists || status.SetupCompleted {
		t.Fatalf("unexpected flags: %+v", status)
	}
}

func TestSetupStatusReportsCompletedWhenEverythingIsConfigured(t *testing.T) {
	setup, _ := newSetupFixture(t, stubInventory{observability: 1, llm: 1})
	if _, err := setup.BootstrapAdmin(context.Background(), "admin@observai.io", "CorrectHorse42"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	status, err := setup.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.State != SetupStateCompleted || !status.SetupCompleted {
		t.Fatalf("expected completed, got %+v", status)
	}
}

func TestSetupBootstrapAdminCreatesAdminUser(t *testing.T) {
	setup, users := newSetupFixture(t, stubInventory{})
	user, err := setup.BootstrapAdminWithOptions(context.Background(), BootstrapAdminRequest{
		Name:     "Gustavo Ferreira",
		Email:    "admin@observai.io",
		Password: "CorrectHorse42",
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if user.Role != domain.RoleAdmin || !user.IsActive {
		t.Fatalf("unexpected admin user: %+v", user)
	}
	if user.Name != "Gustavo Ferreira" {
		t.Fatalf("expected admin name persisted, got %q", user.Name)
	}
	count, _ := users.Count(context.Background())
	if count != 1 {
		t.Fatalf("expected exactly one user after bootstrap, got %d", count)
	}
}

func TestSetupBootstrapAdminAllowsAdditionalAdminWhileSetupIsOpen(t *testing.T) {
	setup, _ := newSetupFixture(t, stubInventory{})
	if _, err := setup.BootstrapAdmin(context.Background(), "admin@observai.io", "CorrectHorse42"); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	user, err := setup.BootstrapAdmin(context.Background(), "another@observai.io", "AnotherP@ss1")
	if err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if user.Email != "another@observai.io" || user.Role != domain.RoleAdmin {
		t.Fatalf("unexpected second admin: %+v", user)
	}
}

func TestSetupBootstrapAdminRejectsWhenSetupCompleted(t *testing.T) {
	setup, _ := newSetupFixture(t, stubInventory{observability: 1, llm: 1})
	if _, err := setup.BootstrapAdmin(context.Background(), "admin@observai.io", "CorrectHorse42"); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	_, err := setup.BootstrapAdmin(context.Background(), "another@observai.io", "AnotherP@ss1")
	if !errors.Is(err, ErrSetupAlreadyCompleted) {
		t.Fatalf("expected ErrSetupAlreadyCompleted, got %v", err)
	}
}

func TestSetupBootstrapAdminValidatesInput(t *testing.T) {
	setup, _ := newSetupFixture(t, stubInventory{})
	_, err := setup.BootstrapAdmin(context.Background(), "", "short")
	if !errors.Is(err, domain.ErrInvalidUser) {
		t.Fatalf("expected ErrInvalidUser, got %v", err)
	}
}
