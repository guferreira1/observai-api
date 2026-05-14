package uuid

import (
	"context"
	"regexp"
	"testing"
)

var uuidV7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNextIDReturnsValidUUIDv7Format(t *testing.T) {
	generator := NewIDGenerator()
	id, err := generator.NextID(context.Background())
	if err != nil {
		t.Fatalf("NextID returned error: %v", err)
	}
	if !uuidV7Pattern.MatchString(id) {
		t.Fatalf("expected UUIDv7 canonical form, got %q", id)
	}
}

func TestNextIDProducesUniqueValuesAcrossManyCalls(t *testing.T) {
	const iterations = 10_000
	generator := NewIDGenerator()
	seen := make(map[string]struct{}, iterations)
	for index := 0; index < iterations; index++ {
		id, err := generator.NextID(context.Background())
		if err != nil {
			t.Fatalf("NextID returned error at iteration %d: %v", index, err)
		}
		if _, duplicated := seen[id]; duplicated {
			t.Fatalf("duplicate identifier produced at iteration %d: %s", index, id)
		}
		seen[id] = struct{}{}
	}
}

func TestNextIDPreservesApproximateMonotonicOrder(t *testing.T) {
	generator := NewIDGenerator()
	previous, err := generator.NextID(context.Background())
	if err != nil {
		t.Fatalf("first NextID returned error: %v", err)
	}
	for index := 0; index < 1_000; index++ {
		current, err := generator.NextID(context.Background())
		if err != nil {
			t.Fatalf("NextID returned error at iteration %d: %v", index, err)
		}
		if current < previous {
			t.Fatalf("UUIDv7 ordering broken at iteration %d: %s came before %s", index, previous, current)
		}
		previous = current
	}
}
