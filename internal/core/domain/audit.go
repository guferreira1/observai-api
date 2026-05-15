package domain

import "time"

// AuditEntry describes a single audit_log row. The shape is intentionally
// broad: it captures both the request-level data populated by the HTTP
// audit middleware and the granular domain event data emitted by use
// cases through AuditLog.Record.
//
// APIKeyID is the persistent identifier of the resolved API key when the
// request authenticated with one. Actor is the human-readable name
// (`static-admin`, an email, or a key alias). Metadata carries an opaque
// JSON object — use cases pass structured key-value pairs that the
// repository serializes as jsonb; secret-shaped values are masked before
// persistence so audit rows never leak credentials.
type AuditEntry struct {
	ID           int64
	RequestID    string
	APIKeyID     string
	Actor        string
	Method       string
	Path         string
	Status       int
	DurationMs   int64
	Remote       string
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]string
	CreatedAt    time.Time
}

// AuditFilter narrows audit_log queries.
type AuditFilter struct {
	APIKeyID     string
	Actor        string
	Action       string
	ResourceType string
	ResourceID   string
	From         time.Time
	To           time.Time
	Limit        int
	Offset       int
}
