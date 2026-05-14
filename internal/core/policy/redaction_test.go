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
	redacted := NewEmailRedactor().Redact(evidence)
	if redacted.Summary == evidence.Summary {
		t.Fatalf("expected summary to be scrubbed, got %q", redacted.Summary)
	}
	if redacted.Attributes["reporter"] == evidence.Attributes["reporter"] {
		t.Fatalf("expected attribute to be scrubbed, got %q", redacted.Attributes["reporter"])
	}
}

func TestBearerTokenRedactorScrubsAuthorizationLines(t *testing.T) {
	evidence := domain.Evidence{Summary: "Authorization: Bearer abc123xyz failure"}
	redacted := NewBearerTokenRedactor().Redact(evidence)
	if redacted.Summary == evidence.Summary {
		t.Fatalf("expected bearer token to be scrubbed, got %q", redacted.Summary)
	}
}

func TestIPv4RedactorScrubsAddresses(t *testing.T) {
	evidence := domain.Evidence{Summary: "request from 10.0.42.7 failed"}
	redacted := NewIPv4Redactor().Redact(evidence)
	if redacted.Summary == evidence.Summary {
		t.Fatalf("expected ipv4 to be scrubbed, got %q", redacted.Summary)
	}
}

func TestChainRedactorAppliesEveryRedactor(t *testing.T) {
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
}

func TestJWTRedactorScrubsCanonicalToken(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	evidence := domain.Evidence{Summary: "auth token " + jwt + " failed"}
	redacted := NewJWTRedactor().Redact(evidence)
	if redacted.Summary == evidence.Summary {
		t.Fatalf("expected JWT to be scrubbed, got %q", redacted.Summary)
	}
}

func TestCreditCardRedactorScrubsLuhnValidNumber(t *testing.T) {
	evidence := domain.Evidence{Summary: "charged card 4111 1111 1111 1111 today"}
	redacted := NewCreditCardRedactor().Redact(evidence)
	if redacted.Summary == evidence.Summary {
		t.Fatalf("expected card to be scrubbed, got %q", redacted.Summary)
	}
}

func TestCreditCardRedactorLeavesLuhnInvalidNumberUntouched(t *testing.T) {
	evidence := domain.Evidence{Summary: "order id 1234567890123456"}
	redacted := NewCreditCardRedactor().Redact(evidence)
	if redacted.Summary != evidence.Summary {
		t.Fatalf("non-Luhn number must not be scrubbed, got %q", redacted.Summary)
	}
}

func TestCPFRedactorScrubsFormattedAndUnformattedCPF(t *testing.T) {
	formatted := domain.Evidence{Summary: "user 123.456.789-09 reported issue"}
	unformatted := domain.Evidence{Summary: "user 12345678909 reported issue"}
	if NewCPFRedactor().Redact(formatted).Summary == formatted.Summary {
		t.Fatalf("formatted CPF must be scrubbed")
	}
	if NewCPFRedactor().Redact(unformatted).Summary == unformatted.Summary {
		t.Fatalf("unformatted CPF must be scrubbed")
	}
}

func TestChainRedactorIsNoOpWithoutRedactors(t *testing.T) {
	chain := NewChainRedactor()
	evidence := []domain.Evidence{{Summary: "anything"}}
	if got := chain.RedactEvidence(evidence); &got[0] != &evidence[0] {
		// We expect the input slice to be returned untouched when nothing to do.
		t.Fatalf("expected pass-through on empty chain")
	}
}
