package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/observability"
	redisclient "github.com/redis/go-redis/v9"
)

const (
	analysisLockKeyPrefix = "observai:chat-lock:v1:"
	defaultLockTTL        = 60 * time.Second
	defaultLockWait       = 30 * time.Second
	lockBackoffMin        = 5 * time.Millisecond
	lockBackoffMax        = 100 * time.Millisecond
)

// luaReleaseScript releases the lock only when the held token matches.
//
// The compare-and-delete is atomic so a slow holder cannot accidentally release
// a lock that already expired and was acquired by a different caller.
var luaReleaseScript = redisclient.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

// ErrLockUnavailable is returned when the configured wait window elapses without acquiring the lock.
var ErrLockUnavailable = errors.New("analysis chat lock unavailable")

// AnalysisLocker serializes critical sections per analysis identifier through Redis.
type AnalysisLocker struct {
	client   *redisclient.Client
	ttl      time.Duration
	wait     time.Duration
	observer observability.ProviderObserver
}

// LockerOptions configures the analysis locker.
type LockerOptions struct {
	TTL      time.Duration
	Wait     time.Duration
	Observer observability.ProviderObserver
}

// NewAnalysisLocker creates a Redis-backed analysis locker.
func NewAnalysisLocker(ctx context.Context, redisURL string, opts LockerOptions) (*AnalysisLocker, error) {
	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redisclient.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	if opts.TTL <= 0 {
		opts.TTL = defaultLockTTL
	}
	if opts.Wait <= 0 {
		opts.Wait = defaultLockWait
	}
	if opts.Observer == nil {
		opts.Observer = observability.NoopProviderObserver{}
	}

	return &AnalysisLocker{
		client:   client,
		ttl:      opts.TTL,
		wait:     opts.Wait,
		observer: opts.Observer,
	}, nil
}

// Ping verifies connectivity to Redis with the supplied context.
func (locker *AnalysisLocker) Ping(ctx context.Context) error {
	return locker.client.Ping(ctx).Err()
}

// Close releases Redis connections held by the locker.
func (locker *AnalysisLocker) Close() error {
	if err := locker.client.Close(); err != nil {
		return fmt.Errorf("close redis locker client: %w", err)
	}
	return nil
}

// Acquire blocks until the lock for analysisID is granted, ctx is canceled or
// the configured wait window elapses.
func (locker *AnalysisLocker) Acquire(ctx context.Context, analysisID string) (release func(), err error) {
	startedAt := time.Now()
	defer func() {
		locker.observer.Observe("redis", "acquire_analysis_lock", time.Since(startedAt), err)
	}()

	token, err := newLockToken()
	if err != nil {
		return nil, fmt.Errorf("generate analysis lock token: %w", err)
	}
	key := analysisLockKey(analysisID)

	deadline := startedAt.Add(locker.wait)
	backoff := lockBackoffMin

	for {
		acquired, err := locker.client.SetNX(ctx, key, token, locker.ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("acquire analysis chat lock: %w", err)
		}
		if acquired {
			return locker.releaseFunc(key, token), nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s", ErrLockUnavailable, analysisID)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > lockBackoffMax {
			backoff = lockBackoffMax
		}
	}
}

func (locker *AnalysisLocker) releaseFunc(key string, token string) func() {
	released := false
	return func() {
		if released {
			return
		}
		released = true
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = luaReleaseScript.Run(releaseCtx, locker.client, []string{key}, token).Result()
	}
}

func analysisLockKey(analysisID string) string {
	return analysisLockKeyPrefix + analysisID
}

func newLockToken() (string, error) {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer[:]), nil
}
