package domain

import "time"

// AuditEntry describes a single authenticated HTTP request observed by the
// audit middleware.
//
// APIKeyID is the persistent identifier of the resolved API key, when
// known. Actor is the human-readable name (e.g. "static-admin" or the
// stored key name). Skipped paths (operational probes) are not audited.
type AuditEntry struct {
	ID         int64
	RequestID  string
	APIKeyID   string
	Actor      string
	Method     string
	Path       string
	Status     int
	DurationMs int64
	Remote     string
	CreatedAt  time.Time
}

// AuditFilter narrows audit_log queries.
type AuditFilter struct {
	APIKeyID string
	From     time.Time
	To       time.Time
	Limit    int
	Offset   int
}
