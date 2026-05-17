package ports

import "time"

// PasswordHasher hashes and verifies user credentials using the
// installed password algorithm.
//
// Implementations isolate the algorithm choice (bcrypt, argon2, ...) from
// the use case layer so the core depends only on the abstract behavior.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hash string, password string) error
}

// AccessTokenClaims is the canonical claim set used by ObservAI access
// tokens.
//
// Subject carries the user identifier, Role carries the authorization
// level, JTI uniquely identifies the token for audit and the standard
// IssuedAt/ExpiresAt fields gate validity. The issuer is enforced by the
// signer configuration and is not exposed per claim.
type AccessTokenClaims struct {
	Subject   string
	Role      string
	JTI       string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// AccessTokenSigner issues and validates short-lived access tokens.
//
// Implementations must collapse signature, expiry and issuer validation
// failures into a single error so callers cannot distinguish failure
// modes that would leak information about the secret or stored claims.
type AccessTokenSigner interface {
	Sign(claims AccessTokenClaims) (string, error)
	Parse(raw string) (AccessTokenClaims, error)
}
