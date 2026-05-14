package ports

import "context"

// CredentialStore resolves opaque references into the actual secret value.
//
// References use a scheme prefix to identify the resolver responsible for
// returning the secret material: "env:OBSERVAI_OPENAI_KEY" reads from the
// process environment, "file:/run/secrets/openai" reads from a mounted
// secret file. Additional schemes (vault, gcp, aws) plug in without
// touching call sites.
//
// Implementations must not return the empty string for a valid reference;
// missing values must be returned as errors so callers fail fast at
// startup rather than silently sending unauthenticated requests.
type CredentialStore interface {
	Resolve(ctx context.Context, reference string) (string, error)
}
