package policy

import (
	"errors"
	"testing"
)

func TestFailFastCollectorErrorPolicyAlwaysAborts(t *testing.T) {
	policy := NewFailFastCollectorErrorPolicy()
	if policy.HandleCollectorError("prometheus", errors.New("boom")) {
		t.Fatal("FailFast policy must abort on any error")
	}
}

func TestPartialFailureCollectorErrorPolicyAlwaysContinues(t *testing.T) {
	policy := NewPartialFailureCollectorErrorPolicy()
	if !policy.HandleCollectorError("loki", errors.New("boom")) {
		t.Fatal("PartialFailure policy must continue on any error")
	}
}
