// Package prompts loads versioned LLM agent prompts from disk.
//
// Runtime LLM behavior must come from files under the public agents/ directory,
// never from local-only documentation under .claude/. The loader caches the
// rendered prompt content per file name and is safe for concurrent use.
package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Loader returns the textual content of named agent prompts.
type Loader interface {
	Load(name string) (string, error)
}

// FileLoader reads agent prompts from a base directory and caches them in memory.
type FileLoader struct {
	baseDir string
	mu      sync.RWMutex
	cache   map[string]string
}

// NewFileLoader creates a loader rooted at baseDir.
func NewFileLoader(baseDir string) *FileLoader {
	return &FileLoader{
		baseDir: baseDir,
		cache:   make(map[string]string),
	}
}

// Load returns the prompt content for the given agent name. Names should not
// contain path separators or relative components.
func (loader *FileLoader) Load(name string) (string, error) {
	if err := validateAgentName(name); err != nil {
		return "", err
	}

	loader.mu.RLock()
	cached, ok := loader.cache[name]
	loader.mu.RUnlock()
	if ok {
		return cached, nil
	}

	path := filepath.Join(loader.baseDir, name+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read agent prompt %q: %w", name, err)
	}

	rendered := strings.TrimSpace(string(content))
	loader.mu.Lock()
	loader.cache[name] = rendered
	loader.mu.Unlock()

	return rendered, nil
}

// Reset clears any cached prompt content. Useful for tests and live reload.
func (loader *FileLoader) Reset() {
	loader.mu.Lock()
	loader.cache = make(map[string]string)
	loader.mu.Unlock()
}

func validateAgentName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("agent prompt name is required")
	}
	if strings.ContainsAny(trimmed, `/\`) || strings.Contains(trimmed, "..") {
		return fmt.Errorf("invalid agent prompt name %q", name)
	}
	return nil
}
