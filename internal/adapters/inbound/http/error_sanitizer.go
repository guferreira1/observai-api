package http

import (
	"regexp"
	"strings"
)

// secretSanitizers strip common credential patterns from arbitrary strings
// before they are surfaced through API responses, audit logs or error
// messages. The list intentionally targets the high-signal cases: URLs that
// embed basic-auth credentials, query-string token parameters and the
// header/keyword forms commonly produced by Go's net/http and provider
// SDK error formatting.
var secretSanitizers = []*regexp.Regexp{
	regexp.MustCompile(`https?://[^\s:/@]+:[^\s/@]+@[^\s"']+`),
	regexp.MustCompile(`(?i)(?:authorization|api[\-_ ]?token|api[\-_ ]?key|x[\-_]api[\-_]key|password|secret)\s*[:=]\s*(?:bearer\s+)?\S+`),
	regexp.MustCompile(`(?i)\b(?:bearer|token)\s+[A-Za-z0-9._\-]{8,}`),
	regexp.MustCompile(`(?i)\b(?:token|api_key|access_token|password)=([^&\s"']+)`),
}

// SanitizeExternalMessage redacts credential-shaped substrings from an
// arbitrary error or upstream message so it can be safely embedded in an
// HTTP response payload, audit log or operator-facing log line.
func SanitizeExternalMessage(message string) string {
	cleaned := message
	for _, sanitizer := range secretSanitizers {
		cleaned = sanitizer.ReplaceAllString(cleaned, "[redacted]")
	}
	return strings.TrimSpace(cleaned)
}
