package testfakes

import (
	"context"
	"fmt"
	"sync/atomic"
)

// IDGenerator creates deterministic monotonic identifiers for tests.
type IDGenerator struct {
	prefix string
	next   atomic.Uint64
}

// NewIDGenerator creates a deterministic identifier generator for tests.
func NewIDGenerator(prefix string) *IDGenerator {
	return &IDGenerator{prefix: prefix}
}

// NextID returns the next deterministic identifier.
func (generator *IDGenerator) NextID(_ context.Context) (string, error) {
	id := generator.next.Add(1)
	return fmt.Sprintf("%s-%06d", generator.prefix, id), nil
}
