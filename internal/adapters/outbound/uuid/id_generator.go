// Package uuid provides a production identifier generator backed by UUIDv7.
//
// UUIDv7 (RFC 9562) embeds a millisecond timestamp in the leading 48 bits,
// which yields k-sortable identifiers that approximate creation order when
// compared lexicographically. The values are unique across instances, which
// matters once analyses can be enqueued by multiple workers consuming the
// shared Redis queue.
package uuid

import (
	"context"
	"fmt"

	googleuuid "github.com/google/uuid"
)

// IDGenerator returns time-ordered UUIDv7 identifiers.
type IDGenerator struct{}

// NewIDGenerator builds a UUIDv7-backed identifier generator.
func NewIDGenerator() *IDGenerator {
	return &IDGenerator{}
}

// NextID returns a fresh UUIDv7 encoded as its canonical string form.
//
// The context is accepted to satisfy ports.IDGenerator and reserved for
// future cancellation; the underlying generator does not block.
func (generator *IDGenerator) NextID(_ context.Context) (string, error) {
	value, err := googleuuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate uuidv7: %w", err)
	}
	return value.String(), nil
}
