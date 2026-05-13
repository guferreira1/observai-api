package prompts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLoaderReadsAndCachesPrompt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "analysis.md"), []byte("system prompt body\n"), 0o600))

	loader := NewFileLoader(dir)
	first, err := loader.Load("analysis")
	require.NoError(t, err)
	assert.Equal(t, "system prompt body", first)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "analysis.md"), []byte("modified content"), 0o600))
	cached, err := loader.Load("analysis")
	require.NoError(t, err)
	assert.Equal(t, "system prompt body", cached, "cached content must survive disk changes until reset")

	loader.Reset()
	updated, err := loader.Load("analysis")
	require.NoError(t, err)
	assert.Equal(t, "modified content", updated)
}

func TestFileLoaderRejectsTraversal(t *testing.T) {
	t.Parallel()

	loader := NewFileLoader(t.TempDir())
	_, err := loader.Load("../etc/passwd")
	assert.Error(t, err)

	_, err = loader.Load("")
	assert.Error(t, err)
}

func TestFileLoaderReturnsErrorForMissingFile(t *testing.T) {
	t.Parallel()

	loader := NewFileLoader(t.TempDir())
	_, err := loader.Load("missing")
	assert.Error(t, err)
}
