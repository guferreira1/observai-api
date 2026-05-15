package policy

import (
	"regexp"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

// EvidenceRedactor scrubs sensitive substrings from a single piece of evidence.
//
// Each redactor is a focused Rule object that knows one redaction concern
// (emails, bearer tokens, IP addresses). The Name returned by the rule is
// stamped on Evidence.RedactedFields when the rule fires so the API can
// expose which redactors actually rewrote data.
type EvidenceRedactor interface {
	Name() string
	Redact(evidence domain.Evidence) (domain.Evidence, bool)
}

// RedactorRule names the standard rules supported by the chain. Operators
// enable/disable individual rules through configuration.
type RedactorRule string

const (
	// RuleBearerToken redacts `Bearer <token>` substrings.
	RuleBearerToken RedactorRule = "bearer"
	// RuleJWT redacts JSON Web Tokens.
	RuleJWT RedactorRule = "jwt"
	// RuleEmail redacts email addresses.
	RuleEmail RedactorRule = "email"
	// RuleIPv4 redacts IPv4 literals.
	RuleIPv4 RedactorRule = "ipv4"
)

// AllRedactorRules lists the rules wired into the default chain.
func AllRedactorRules() []RedactorRule {
	return []RedactorRule{RuleBearerToken, RuleJWT, RuleEmail, RuleIPv4}
}

// IsValidRedactorRule reports whether rule is a known value.
func IsValidRedactorRule(rule RedactorRule) bool {
	for _, candidate := range AllRedactorRules() {
		if candidate == rule {
			return true
		}
	}
	return false
}

// RedactorConfig drives which rules participate in the default chain. When
// Enabled is empty, every rule in AllRedactorRules() is included; otherwise
// only the listed rules run, in the order specified.
type RedactorConfig struct {
	Enabled []RedactorRule
}

// ChainRedactor applies a list of EvidenceRedactor in order to every
// Evidence item it receives.
type ChainRedactor struct {
	redactors []EvidenceRedactor
}

// NewChainRedactor builds a chain that applies the supplied redactors in
// order. The zero-value chain is a no-op; pass an empty slice to disable
// redaction.
func NewChainRedactor(redactors ...EvidenceRedactor) ChainRedactor {
	return ChainRedactor{redactors: redactors}
}

// NewDefaultRedactor returns a chain that scrubs the most common high-risk
// substrings using the standard rule order.
func NewDefaultRedactor() ChainRedactor {
	return NewRedactorFromConfig(RedactorConfig{})
}

// NewRedactorFromConfig builds a chain from the operator-supplied config.
// Unknown rules are ignored so a typo cannot silently disable an active
// chain.
func NewRedactorFromConfig(config RedactorConfig) ChainRedactor {
	rules := config.Enabled
	if len(rules) == 0 {
		rules = AllRedactorRules()
	}
	registry := defaultRedactorRegistry()
	chain := make([]EvidenceRedactor, 0, len(rules))
	for _, rule := range rules {
		if redactor, ok := registry[rule]; ok {
			chain = append(chain, redactor)
		}
	}
	return ChainRedactor{redactors: chain}
}

// Rules returns the rule names registered on the chain. Useful for the
// HTTP response metadata so clients can advertise which rules participate.
func (chain ChainRedactor) Rules() []string {
	if len(chain.redactors) == 0 {
		return nil
	}
	rules := make([]string, 0, len(chain.redactors))
	for _, redactor := range chain.redactors {
		rules = append(rules, redactor.Name())
	}
	return rules
}

// RedactEvidence returns the supplied evidence with every redactor applied.
//
// Each evidence accumulates a RedactedFields entry per rule that actually
// rewrote one of its strings, so callers can surface "this was masked"
// information without parsing the redacted output.
func (chain ChainRedactor) RedactEvidence(evidence []domain.Evidence) []domain.Evidence {
	if len(chain.redactors) == 0 || len(evidence) == 0 {
		return evidence
	}
	redacted := make([]domain.Evidence, len(evidence))
	for index, item := range evidence {
		for _, redactor := range chain.redactors {
			updated, fired := redactor.Redact(item)
			item = updated
			if fired {
				item.RedactedFields = appendUnique(item.RedactedFields, redactor.Name())
			}
		}
		redacted[index] = item
	}
	return redacted
}

func defaultRedactorRegistry() map[RedactorRule]EvidenceRedactor {
	return map[RedactorRule]EvidenceRedactor{
		RuleBearerToken: NewBearerTokenRedactor(),
		RuleJWT:         NewJWTRedactor(),
		RuleEmail:       NewEmailRedactor(),
		RuleIPv4:        NewIPv4Redactor(),
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// emailPattern matches the conservative subset of email addresses we redact.
var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

// bearerPattern matches `Bearer <token>` and `Authorization: Bearer <token>` snippets.
var bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`)

// ipv4Pattern matches IPv4 literals.
var ipv4Pattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

// jwtPattern matches the canonical JWT shape header.payload.signature where
// each segment is base64url-encoded.
var jwtPattern = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\b`)

// EmailRedactor replaces email addresses with [redacted-email].
type EmailRedactor struct{}

// NewEmailRedactor builds an EmailRedactor.
func NewEmailRedactor() EmailRedactor { return EmailRedactor{} }

// Name returns the rule identifier.
func (EmailRedactor) Name() string { return string(RuleEmail) }

// Redact implements EvidenceRedactor.
func (EmailRedactor) Redact(evidence domain.Evidence) (domain.Evidence, bool) {
	fired := false
	evidence.Summary, fired = replaceAndTrack(evidence.Summary, emailPattern, "[redacted-email]", fired)
	evidence.Name, fired = replaceAndTrack(evidence.Name, emailPattern, "[redacted-email]", fired)
	evidence.Reference, fired = replaceAndTrack(evidence.Reference, emailPattern, "[redacted-email]", fired)
	evidence.Attributes, fired = scrubAttributes(evidence.Attributes, emailPattern, "[redacted-email]", fired)
	return evidence, fired
}

// BearerTokenRedactor replaces `Bearer <token>` patterns with [redacted-bearer].
type BearerTokenRedactor struct{}

// NewBearerTokenRedactor builds a BearerTokenRedactor.
func NewBearerTokenRedactor() BearerTokenRedactor { return BearerTokenRedactor{} }

// Name returns the rule identifier.
func (BearerTokenRedactor) Name() string { return string(RuleBearerToken) }

// Redact implements EvidenceRedactor.
func (BearerTokenRedactor) Redact(evidence domain.Evidence) (domain.Evidence, bool) {
	fired := false
	evidence.Summary, fired = replaceAndTrack(evidence.Summary, bearerPattern, "[redacted-bearer]", fired)
	evidence.Reference, fired = replaceAndTrack(evidence.Reference, bearerPattern, "[redacted-bearer]", fired)
	evidence.Attributes, fired = scrubAttributes(evidence.Attributes, bearerPattern, "[redacted-bearer]", fired)
	return evidence, fired
}

// IPv4Redactor replaces IPv4 literals with [redacted-ip].
type IPv4Redactor struct{}

// NewIPv4Redactor builds an IPv4Redactor.
func NewIPv4Redactor() IPv4Redactor { return IPv4Redactor{} }

// Name returns the rule identifier.
func (IPv4Redactor) Name() string { return string(RuleIPv4) }

// Redact implements EvidenceRedactor.
func (IPv4Redactor) Redact(evidence domain.Evidence) (domain.Evidence, bool) {
	fired := false
	evidence.Summary, fired = replaceAndTrack(evidence.Summary, ipv4Pattern, "[redacted-ip]", fired)
	evidence.Reference, fired = replaceAndTrack(evidence.Reference, ipv4Pattern, "[redacted-ip]", fired)
	evidence.Attributes, fired = scrubAttributes(evidence.Attributes, ipv4Pattern, "[redacted-ip]", fired)
	return evidence, fired
}

// JWTRedactor replaces JSON Web Tokens with [redacted-jwt].
type JWTRedactor struct{}

// NewJWTRedactor builds a JWTRedactor.
func NewJWTRedactor() JWTRedactor { return JWTRedactor{} }

// Name returns the rule identifier.
func (JWTRedactor) Name() string { return string(RuleJWT) }

// Redact implements EvidenceRedactor.
func (JWTRedactor) Redact(evidence domain.Evidence) (domain.Evidence, bool) {
	fired := false
	evidence.Summary, fired = replaceAndTrack(evidence.Summary, jwtPattern, "[redacted-jwt]", fired)
	evidence.Reference, fired = replaceAndTrack(evidence.Reference, jwtPattern, "[redacted-jwt]", fired)
	evidence.Attributes, fired = scrubAttributes(evidence.Attributes, jwtPattern, "[redacted-jwt]", fired)
	return evidence, fired
}

func replaceAndTrack(value string, pattern *regexp.Regexp, replacement string, alreadyFired bool) (string, bool) {
	if value == "" {
		return value, alreadyFired
	}
	if !pattern.MatchString(value) {
		return value, alreadyFired
	}
	return pattern.ReplaceAllString(value, replacement), true
}

func scrubAttributes(attributes map[string]string, pattern *regexp.Regexp, replacement string, alreadyFired bool) (map[string]string, bool) {
	if len(attributes) == 0 {
		return attributes, alreadyFired
	}
	fired := alreadyFired
	scrubbed := make(map[string]string, len(attributes))
	for key, value := range attributes {
		updated, matched := replaceAndTrack(value, pattern, replacement, false)
		if matched {
			fired = true
		}
		scrubbed[key] = updated
	}
	return scrubbed, fired
}
