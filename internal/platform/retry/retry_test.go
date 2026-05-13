package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoStopsOnNonRetryableError(t *testing.T) {
	t.Parallel()

	calls := 0
	policy := Policy{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
	err := Do(context.Background(), policy, func(error) bool { return false }, func(attempt int) error {
		calls++
		return errors.New("permanent failure")
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestDoRetriesUntilSuccess(t *testing.T) {
	t.Parallel()

	calls := 0
	policy := Policy{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
	err := Do(context.Background(), policy, func(error) bool { return true }, func(attempt int) error {
		calls++
		if attempt < 3 {
			return errors.New("transient failure")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestDoRespectsMaxAttempts(t *testing.T) {
	t.Parallel()

	calls := 0
	policy := Policy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
	err := Do(context.Background(), policy, func(error) bool { return true }, func(attempt int) error {
		calls++
		return errors.New("transient")
	})

	require.Error(t, err)
	assert.Equal(t, 2, calls)
}

func TestDoStopsOnContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	policy := Policy{MaxAttempts: 5, BaseDelay: 50 * time.Millisecond, MaxDelay: 50 * time.Millisecond}

	err := Do(ctx, policy, func(error) bool { return true }, func(attempt int) error {
		calls++
		if attempt == 1 {
			cancel()
		}
		return errors.New("transient")
	})

	require.Error(t, err)
	assert.LessOrEqual(t, calls, 2)
}
