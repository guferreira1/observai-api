// Package testfakes hosts deterministic outbound adapters intended for unit
// and integration tests only.
//
// Production code must not import this package. The HTTP composition root
// (cmd/observai-api) selects real provider adapters or the null adapters in
// internal/adapters/outbound/null when an operator has not configured a
// backend. These doubles existed historically as fake adapters that
// silently fabricated observability data, which was unsafe in production;
// keeping them behind a clearly named test-only package preserves their
// value for tests without inviting accidental runtime use.
package testfakes
