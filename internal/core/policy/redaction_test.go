package policy

import (
	"testing"

	"github.com/guferreira1/observai-api/internal/core/domain"
)

func TestEmailRedactorScrubsSummaryAndAttributes(t *testing.T) {
	evidence := domain.Evidence{
		Summary:    "user alice@example.com saw an error",
		Attributes: map[string]string{"reporter": "bob.smith+work@corp.example"},
	}
	redacted, fired := NewEmailRedactor().Redact(evidence)
	if !fired {
		t.Fatalf("expected redactor to report fired=true")
	}
	if redacted.Summary == evidence.Summary {
		t.Fatalf("expected summary to be scrubbed, got %q", redacted.Summary)
	}
	if redacted.Attributes["reporter"] == evidence.Attributes["reporter"] {
		t.Fatalf("expected attribute to be scrubbed, got %q", redacted.Attributes["reporter"])
	}
}

func TestBearerTokenRedactorScrubsAuthorizationLines(t *testing.T) {
	evidence := domain.Evidence{Summary: "Authorization: Bearer abc123xyz failure"}
	redacted, fired := NewBearerTokenRedactor().Redact(evidence)
	if !fired || redacted.Summary == evidence.Summary {
		t.Fatalf("expected bearer token to be scrubbed, got fired=%v summary=%q", fired, redacted.Summary)
	}
}

func TestIPv4RedactorScrubsAddresses(t *testing.T) {
	evidence := domain.Evidence{Summary: "request from 10.0.42.7 failed"}
	redacted, fired := NewIPv4Redactor().Redact(evidence)
	if !fired || redacted.Summary == evidence.Summary {
		t.Fatalf("expected ipv4 to be scrubbed, got fired=%v summary=%q", fired, redacted.Summary)
	}
}

func TestChainRedactorAppliesEveryRedactorAndStampsFields(t *testing.T) {
	chain := NewDefaultRedactor()
	original := domain.Evidence{
		Summary: "alice@example.com (10.0.0.1) used Bearer abcd token",
	}
	redacted := chain.RedactEvidence([]domain.Evidence{original})
	if len(redacted) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(redacted))
	}
	scrubbed := redacted[0].Summary
	if scrubbed == original.Summary {
		t.Fatalf("expected chain to mutate the summary, got %q", scrubbed)
	}
	fields := redacted[0].RedactedFields
	if !containsString(fields, string(RuleBearerToken)) {
		t.Fatalf("expected bearer token rule to be recorded, got %v", fields)
	}
	if !containsString(fields, string(RuleEmail)) {
		t.Fatalf("expected email rule to be recorded, got %v", fields)
	}
	if !containsString(fields, string(RuleIPv4)) {
		t.Fatalf("expected ipv4 rule to be recorded, got %v", fields)
	}
}

func TestChainRedactorDoesNotStampRulesThatDidNotFire(t *testing.T) {
	chain := NewDefaultRedactor()
	evidence := []domain.Evidence{{Summary: "no secrets here"}}
	redacted := chain.RedactEvidence(evidence)
	if len(redacted[0].RedactedFields) != 0 {
		t.Fatalf("expected no rules recorded on clean evidence, got %v", redacted[0].RedactedFields)
	}
}

func TestJWTRedactorScrubsCanonicalToken(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	evidence := domain.Evidence{Summary: "auth token " + jwt + " failed"}
	redacted, fired := NewJWTRedactor().Redact(evidence)
	if !fired || redacted.Summary == evidence.Summary {
		t.Fatalf("expected JWT to be scrubbed, got fired=%v summary=%q", fired, redacted.Summary)
	}
}

func TestCreditCardRedactorScrubsLuhnValidNumber(t *testing.T) {
	evidence := domain.Evidence{Summary: "charged card 4111 1111 1111 1111 today"}
	redacted, fired := NewCreditCardRedactor().Redact(evidence)
	if !fired || redacted.Summary == evidence.Summary {
		t.Fatalf("expected card to be scrubbed, got fired=%v summary=%q", fired, redacted.Summary)
	}
}

func TestCreditCardRedactorLeavesLuhnInvalidNumberUntouched(t *testing.T) {
	evidence := domain.Evidence{Summary: "order id 1234567890123456"}
	redacted, fired := NewCreditCardRedactor().Redact(evidence)
	if fired || redacted.Summary != evidence.Summary {
		t.Fatalf("non-Luhn number must not be scrubbed, got fired=%v summary=%q", fired, redacted.Summary)
	}
}

func TestCPFRedactorScrubsFormattedAndUnformattedCPF(t *testing.T) {
	formatted := domain.Evidence{Summary: "user 123.456.789-09 reported issue"}
	unformatted := domain.Evidence{Summary: "user 12345678909 reported issue"}
	if redacted, fired := NewCPFRedactor().Redact(formatted); !fired || redacted.Summary == formatted.Summary {
		t.Fatalf("formatted CPF must be scrubbed: fired=%v summary=%q", fired, redacted.Summary)
	}
	if redacted, fired := NewCPFRedactor().Redact(unformatted); !fired || redacted.Summary == unformatted.Summary {
		t.Fatalf("unformatted CPF must be scrubbed: fired=%v summary=%q", fired, redacted.Summary)
	}
}

func TestChainRedactorIsNoOpWithoutRedactors(t *testing.T) {
	chain := NewChainRedactor()
	evidence := []domain.Evidence{{Summary: "anything"}}
	if got := chain.RedactEvidence(evidence); &got[0] != &evidence[0] {
		t.Fatalf("expected pass-through on empty chain")
	}
}

func TestNewRedactorFromConfigRespectsEnabledList(t *testing.T) {
	chain := NewRedactorFromConfig(RedactorConfig{Enabled: []RedactorRule{RuleEmail}})
	rules := chain.Rules()
	if len(rules) != 1 || rules[0] != string(RuleEmail) {
		t.Fatalf("expected only the email rule, got %v", rules)
	}
	evidence := []domain.Evidence{{Summary: "alice@example.com Bearer abc"}}
	redacted := chain.RedactEvidence(evidence)
	if containsString(redacted[0].RedactedFields, string(RuleBearerToken)) {
		t.Fatalf("disabled rule must not fire: %v", redacted[0].RedactedFields)
	}
	if !containsString(redacted[0].RedactedFields, string(RuleEmail)) {
		t.Fatalf("enabled rule must fire: %v", redacted[0].RedactedFields)
	}
}

func TestNewRedactorFromConfigIgnoresUnknownRules(t *testing.T) {
	chain := NewRedactorFromConfig(RedactorConfig{Enabled: []RedactorRule{"bogus", RuleIPv4}})
	rules := chain.Rules()
	if len(rules) != 1 || rules[0] != string(RuleIPv4) {
		t.Fatalf("unknown rules must be ignored, got %v", rules)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
