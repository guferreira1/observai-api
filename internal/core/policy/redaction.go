package policy

import (
	"regexp"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// EvidenceRedactor scrubs sensitive substrings from a single piece of evidence.
//
// Each redactor is a focused Rule object that knows one redaction concern
// (emails, bearer tokens, IP addresses). Composing them via ChainRedactor
// keeps the responsibility per implementation small and tests focused.
type EvidenceRedactor interface {
	Redact(evidence domain.Evidence) domain.Evidence
}

// ChainRedactor applies a list of EvidenceRedactor in order to every
// Evidence item it receives. It satisfies ports.EvidenceRedactor.
type ChainRedactor struct {
	redactors []EvidenceRedactor
}

// NewChainRedactor builds a chain that applies the supplied redactors in
// order. The zero-value chain is a no-op; pass nil to disable redaction at
// the use case boundary.
func NewChainRedactor(redactors ...EvidenceRedactor) ChainRedactor {
	return ChainRedactor{redactors: redactors}
}

// NewDefaultRedactor returns a chain that scrubs the most common high-risk
// substrings: bearer tokens, JWTs, emails, IPv4 addresses, credit cards
// and Brazilian CPF numbers.
//
// Bearer/JWT redactors run before email/IP so an Authorization header
// containing `Bearer eyJ...` becomes `[redacted-bearer]` instead of
// `[redacted-jwt]` (the more specific match wins for tokens, the more
// specific match wins for IDs).
func NewDefaultRedactor() ChainRedactor {
	return NewChainRedactor(
		NewBearerTokenRedactor(),
		NewJWTRedactor(),
		NewEmailRedactor(),
		NewIPv4Redactor(),
		NewCreditCardRedactor(),
		NewCPFRedactor(),
	)
}

// RedactEvidence returns the supplied evidence with every redactor applied.
//
// The function is safe to call with an empty chain (returns the input
// untouched) so the analysis use case can always invoke it without first
// checking for configuration.
func (chain ChainRedactor) RedactEvidence(evidence []domain.Evidence) []domain.Evidence {
	if len(chain.redactors) == 0 || len(evidence) == 0 {
		return evidence
	}
	redacted := make([]domain.Evidence, len(evidence))
	for index, item := range evidence {
		for _, redactor := range chain.redactors {
			item = redactor.Redact(item)
		}
		redacted[index] = item
	}
	return redacted
}

// emailPattern matches the conservative subset of email addresses we redact.
var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// bearerPattern matches `Bearer <token>` and `Authorization: Bearer <token>` snippets.
var bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`)

// ipv4Pattern matches IPv4 literals. The bounds check is intentional: it
// would be cleaner with `\b`, but Go's RE2 lacks lookarounds and the worst
// case (matching 999.999.999.999 substrings) is acceptable for redaction.
var ipv4Pattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

// jwtPattern matches the canonical JWT shape header.payload.signature where
// each segment is base64url-encoded. It is intentionally conservative
// (length ≥ 8 per segment) to avoid scrubbing unrelated dot-separated text.
var jwtPattern = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\b`)

// creditCardPattern matches 13-19 digit sequences that can be separated by
// spaces or dashes (the common visual formats). Luhn validation happens in
// CreditCardRedactor.Redact so false positives on long numeric IDs are rare.
var creditCardPattern = regexp.MustCompile(`\b(?:\d[ \-]?){12,18}\d\b`)

// cpfPattern matches Brazilian CPF in xxx.xxx.xxx-xx or 11-digit form.
var cpfPattern = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)

// EmailRedactor replaces email addresses with [redacted-email].
type EmailRedactor struct{}

// NewEmailRedactor builds an EmailRedactor.
func NewEmailRedactor() EmailRedactor { return EmailRedactor{} }

// Redact implements EvidenceRedactor.
func (EmailRedactor) Redact(evidence domain.Evidence) domain.Evidence {
	evidence.Summary = emailPattern.ReplaceAllString(evidence.Summary, "[redacted-email]")
	evidence.Name = emailPattern.ReplaceAllString(evidence.Name, "[redacted-email]")
	evidence.Reference = emailPattern.ReplaceAllString(evidence.Reference, "[redacted-email]")
	evidence.Attributes = scrubAttributes(evidence.Attributes, emailPattern, "[redacted-email]")
	return evidence
}

// BearerTokenRedactor replaces `Bearer <token>` patterns with [redacted-bearer].
type BearerTokenRedactor struct{}

// NewBearerTokenRedactor builds a BearerTokenRedactor.
func NewBearerTokenRedactor() BearerTokenRedactor { return BearerTokenRedactor{} }

// Redact implements EvidenceRedactor.
func (BearerTokenRedactor) Redact(evidence domain.Evidence) domain.Evidence {
	evidence.Summary = bearerPattern.ReplaceAllString(evidence.Summary, "[redacted-bearer]")
	evidence.Reference = bearerPattern.ReplaceAllString(evidence.Reference, "[redacted-bearer]")
	evidence.Attributes = scrubAttributes(evidence.Attributes, bearerPattern, "[redacted-bearer]")
	return evidence
}

// IPv4Redactor replaces IPv4 literals with [redacted-ip].
type IPv4Redactor struct{}

// NewIPv4Redactor builds an IPv4Redactor.
func NewIPv4Redactor() IPv4Redactor { return IPv4Redactor{} }

// Redact implements EvidenceRedactor.
func (IPv4Redactor) Redact(evidence domain.Evidence) domain.Evidence {
	evidence.Summary = ipv4Pattern.ReplaceAllString(evidence.Summary, "[redacted-ip]")
	evidence.Reference = ipv4Pattern.ReplaceAllString(evidence.Reference, "[redacted-ip]")
	evidence.Attributes = scrubAttributes(evidence.Attributes, ipv4Pattern, "[redacted-ip]")
	return evidence
}

// JWTRedactor replaces JSON Web Tokens with [redacted-jwt].
type JWTRedactor struct{}

// NewJWTRedactor builds a JWTRedactor.
func NewJWTRedactor() JWTRedactor { return JWTRedactor{} }

// Redact implements EvidenceRedactor.
func (JWTRedactor) Redact(evidence domain.Evidence) domain.Evidence {
	evidence.Summary = jwtPattern.ReplaceAllString(evidence.Summary, "[redacted-jwt]")
	evidence.Reference = jwtPattern.ReplaceAllString(evidence.Reference, "[redacted-jwt]")
	evidence.Attributes = scrubAttributes(evidence.Attributes, jwtPattern, "[redacted-jwt]")
	return evidence
}

// CreditCardRedactor replaces Luhn-valid card-shaped digit sequences with [redacted-card].
//
// Luhn validation reduces false positives on order numbers and other long
// numeric identifiers while still scrubbing common card formats (with or
// without spaces / dashes).
type CreditCardRedactor struct{}

// NewCreditCardRedactor builds a CreditCardRedactor.
func NewCreditCardRedactor() CreditCardRedactor { return CreditCardRedactor{} }

// Redact implements EvidenceRedactor.
func (CreditCardRedactor) Redact(evidence domain.Evidence) domain.Evidence {
	evidence.Summary = scrubCreditCard(evidence.Summary)
	evidence.Reference = scrubCreditCard(evidence.Reference)
	if len(evidence.Attributes) > 0 {
		scrubbed := make(map[string]string, len(evidence.Attributes))
		for key, value := range evidence.Attributes {
			scrubbed[key] = scrubCreditCard(value)
		}
		evidence.Attributes = scrubbed
	}
	return evidence
}

// CPFRedactor replaces Brazilian CPF numbers with [redacted-cpf].
type CPFRedactor struct{}

// NewCPFRedactor builds a CPFRedactor.
func NewCPFRedactor() CPFRedactor { return CPFRedactor{} }

// Redact implements EvidenceRedactor.
func (CPFRedactor) Redact(evidence domain.Evidence) domain.Evidence {
	evidence.Summary = cpfPattern.ReplaceAllString(evidence.Summary, "[redacted-cpf]")
	evidence.Reference = cpfPattern.ReplaceAllString(evidence.Reference, "[redacted-cpf]")
	evidence.Attributes = scrubAttributes(evidence.Attributes, cpfPattern, "[redacted-cpf]")
	return evidence
}

func scrubCreditCard(value string) string {
	return creditCardPattern.ReplaceAllStringFunc(value, func(match string) string {
		digits := keepDigits(match)
		if len(digits) < 13 || len(digits) > 19 {
			return match
		}
		if !luhnValid(digits) {
			return match
		}
		return "[redacted-card]"
	})
}

func keepDigits(value string) string {
	out := make([]byte, 0, len(value))
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= '0' && character <= '9' {
			out = append(out, character)
		}
	}
	return string(out)
}

func luhnValid(digits string) bool {
	sum := 0
	double := false
	for index := len(digits) - 1; index >= 0; index-- {
		digit := int(digits[index] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}

func scrubAttributes(attributes map[string]string, pattern *regexp.Regexp, replacement string) map[string]string {
	if len(attributes) == 0 {
		return attributes
	}
	scrubbed := make(map[string]string, len(attributes))
	for key, value := range attributes {
		scrubbed[key] = pattern.ReplaceAllString(value, replacement)
	}
	return scrubbed
}
