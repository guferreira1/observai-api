package credentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDispatcherResolvesEnvScheme(t *testing.T) {
	t.Setenv("OBSERVAI_TEST_CREDENTIAL", "topsecret")
	dispatcher := NewDispatcher()

	value, err := dispatcher.Resolve(context.Background(), "env:OBSERVAI_TEST_CREDENTIAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "topsecret" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestDispatcherResolvesFileScheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("api-key\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	dispatcher := NewDispatcher()

	value, err := dispatcher.Resolve(context.Background(), "file:"+path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "api-key" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestDispatcherTreatsBareValueAsLiteral(t *testing.T) {
	dispatcher := NewDispatcher()

	value, err := dispatcher.Resolve(context.Background(), "plaintext-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "plaintext-token" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestDispatcherReturnsErrUnknownSchemeForUnregistered(t *testing.T) {
	dispatcher := NewDispatcher()

	_, err := dispatcher.Resolve(context.Background(), "vault:secret/token")
	if !errors.Is(err, ErrUnknownScheme) {
		t.Fatalf("expected ErrUnknownScheme, got %v", err)
	}
}

func TestDispatcherReturnsErrEmptyReferenceForBlankInput(t *testing.T) {
	dispatcher := NewDispatcher()

	_, err := dispatcher.Resolve(context.Background(), "  ")
	if !errors.Is(err, ErrEmptyReference) {
		t.Fatalf("expected ErrEmptyReference, got %v", err)
	}
}
