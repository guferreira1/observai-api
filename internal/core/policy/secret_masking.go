package policy

import "strings"

// MaskSecret returns a masked representation of a secret value safe to
// display in operator-facing UIs and audit logs.
//
// Short values are fully hidden so leading characters never leak. Longer
// values keep a small prefix and suffix for visual recognition, with the
// middle replaced by asterisks (e.g. "sk-prod-supersecret" -> "sk-****cret").
// The result is purely cosmetic and must never be reused as authentication
// material.
func MaskSecret(secret string) string {
	cleaned := strings.TrimSpace(secret)
	length := len(cleaned)
	switch {
	case length == 0:
		return ""
	case length <= 4:
		return strings.Repeat("*", length)
	case length <= 12:
		return cleaned[:1] + strings.Repeat("*", length-1)
	default:
		return cleaned[:3] + strings.Repeat("*", 4) + cleaned[length-4:]
	}
}
