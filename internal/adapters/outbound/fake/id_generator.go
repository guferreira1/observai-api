package fake

import (
	"context"
	"fmt"
	"sync/atomic"
)

// IDGenerator creates deterministic identifiers for local execution and tests.
type IDGenerator struct {
	prefix string
	next   atomic.Uint64
}

// NewIDGenerator creates a deterministic identifier generator.
func NewIDGenerator(prefix string) *IDGenerator {
	return &IDGenerator{prefix: prefix}
}

// NextID returns the next deterministic identifier.
func (generator *IDGenerator) NextID(_ context.Context) (string, error) {
	id := generator.next.Add(1)
	return fmt.Sprintf("%s-%06d", generator.prefix, id), nil
}
