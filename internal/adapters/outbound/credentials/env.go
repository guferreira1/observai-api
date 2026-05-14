package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// EnvResolver reads secrets from the process environment.
type EnvResolver struct{}

// Resolve returns the value of the supplied environment variable.
func (EnvResolver) Resolve(_ context.Context, name string) (string, error) {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return "", fmt.Errorf("env credential reference requires a variable name")
	}
	value, ok := os.LookupEnv(cleaned)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", cleaned)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("environment variable %q is empty", cleaned)
	}
	return value, nil
}
