package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guferreira1/observai-api/internal/core/domain"
	"github.com/guferreira1/observai-api/internal/core/policy"
	"github.com/guferreira1/observai-api/internal/core/ports"
)

// AuditLog is the use case for persisting and listing audit entries.
type AuditLog struct {
	repository ports.AuditLogRepository
	now        func() time.Time
}

// NewAuditLog creates an AuditLog use case.
func NewAuditLog(repository ports.AuditLogRepository) *AuditLog {
	return &AuditLog{repository: repository, now: time.Now}
}

// AuditEvent is the granular per-domain event payload accepted by Record.
//
// Action follows the `resource.verb` convention (`api_key.created`,
// `provider.activated`, `auth.login_failed`). Actor is the human-readable
// name; ActorID is the persistent user or api-key identifier when known.
// Metadata is a free-form key/value map persisted as jsonb; keys that
// look like secrets are masked at Record time.
type AuditEvent struct {
	RequestID    string
	ActorID      string
	Actor        string
	Action       string
	ResourceType string
	ResourceID   string
	Status       int
	Metadata     map[string]string
}

// secretKeyMarkers lists case-insensitive substrings used to recognise
// metadata keys whose value carries a credential and must be masked
// before persistence.
var secretKeyMarkers = []string{"secret", "token", "password", "api_key", "apikey", "credential"}

// Append persists a single legacy HTTP audit entry produced by the audit
// middleware. The timestamp is overridden with the use case clock when
// the caller did not provide one.
func (useCase *AuditLog) Append(ctx context.Context, entry domain.AuditEntry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = useCase.now().UTC()
	}
	entry.Metadata = maskSecretMetadata(entry.Metadata)
	if err := useCase.repository.Append(ctx, entry); err != nil {
		return fmt.Errorf("append audit entry: %w", err)
	}
	return nil
}

// Record persists a granular domain event. Secret-shaped metadata values
// are masked before reaching the repository so the audit trail never
// stores plaintext credentials.
func (useCase *AuditLog) Record(ctx context.Context, event AuditEvent) error {
	entry := domain.AuditEntry{
		RequestID:    event.RequestID,
		APIKeyID:     event.ActorID,
		Actor:        event.Actor,
		Status:       event.Status,
		Action:       strings.TrimSpace(event.Action),
		ResourceType: strings.TrimSpace(event.ResourceType),
		ResourceID:   strings.TrimSpace(event.ResourceID),
		Metadata:     maskSecretMetadata(event.Metadata),
		CreatedAt:    useCase.now().UTC(),
	}
	if err := useCase.repository.Append(ctx, entry); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

// List returns the most recent audit entries honoring the filter.
func (useCase *AuditLog) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEntry, error) {
	return useCase.repository.List(ctx, filter)
}

func maskSecretMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return metadata
	}
	masked := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if looksLikeSecretKey(key) {
			masked[key] = policy.MaskSecret(value)
			continue
		}
		masked[key] = value
	}
	return masked
}

func looksLikeSecretKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, marker := range secretKeyMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
