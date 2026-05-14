package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// FileResolver reads secrets from files on disk.
//
// Mounted secret files (Docker secrets, Kubernetes projected volumes,
// systemd-credentials) typically expose credentials as the entire file
// contents. The resolver strips trailing whitespace because tools commonly
// append a newline that would otherwise corrupt the credential.
type FileResolver struct{}

// NewFileResolver builds a FileResolver.
func NewFileResolver() FileResolver {
	return FileResolver{}
}

// Resolve reads the file at the supplied path and returns its trimmed contents.
func (FileResolver) Resolve(_ context.Context, path string) (string, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return "", fmt.Errorf("file credential reference requires a path")
	}
	contents, err := os.ReadFile(cleaned)
	if err != nil {
		return "", fmt.Errorf("read credential file %q: %w", cleaned, err)
	}
	value := strings.TrimRight(string(contents), " \t\r\n")
	if value == "" {
		return "", fmt.Errorf("credential file %q is empty", cleaned)
	}
	return value, nil
}
