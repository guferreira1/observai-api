package crypto

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/guferreira1/observai-api/internal/core/ports"
)

// ErrInvalidJWTSecret indicates the supplied HMAC secret is empty or too short.
var ErrInvalidJWTSecret = errors.New("jwt secret must be at least 32 bytes")

// ErrInvalidJWT indicates a token signature is invalid, the token has expired
// or its claims could not be parsed.
var ErrInvalidJWT = errors.New("invalid jwt")

// MinJWTSecretLength is the minimum HMAC secret length, in bytes.
const MinJWTSecretLength = 32

// JWTClaims is the canonical claim set used by ObservAI access tokens.
//
// It is an alias of ports.AccessTokenClaims so the JWT signer implements
// the AccessTokenSigner port without an additional conversion layer.
type JWTClaims = ports.AccessTokenClaims

// JWTSigner issues and validates HS256 access tokens.
//
// JWTSigner implements ports.AccessTokenSigner so use cases can depend on
// the port instead of this concrete adapter.
type JWTSigner struct {
	secret []byte
	issuer string
}

// NewJWTSigner returns a signer backed by the supplied HMAC secret.
//
// The secret must be at least MinJWTSecretLength bytes; shorter secrets are
// rejected to keep brute-force search above the AES-256 target. Issuer is
// embedded as the "iss" claim and verified at parse time when non-empty.
func NewJWTSigner(secret []byte, issuer string) (*JWTSigner, error) {
	if len(secret) < MinJWTSecretLength {
		return nil, ErrInvalidJWTSecret
	}
	clone := make([]byte, len(secret))
	copy(clone, secret)
	return &JWTSigner{secret: clone, issuer: strings.TrimSpace(issuer)}, nil
}

// Sign serializes the supplied claims into a compact HS256 JWT.
func (signer *JWTSigner) Sign(claims JWTClaims) (string, error) {
	standard := jwt.MapClaims{
		"sub":  claims.Subject,
		"role": claims.Role,
		"jti":  claims.JTI,
		"iat":  claims.IssuedAt.Unix(),
		"exp":  claims.ExpiresAt.Unix(),
	}
	if signer.issuer != "" {
		standard["iss"] = signer.issuer
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, standard)
	signed, err := token.SignedString(signer.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

// Parse validates the signature, expiry and (when configured) issuer of the
// supplied token and returns the embedded claims.
//
// Any signature or claim validation failure collapses into ErrInvalidJWT so
// callers cannot distinguish failure modes that would otherwise leak
// information about the secret or the stored claim shape.
func (signer *JWTSigner) Parse(raw string) (JWTClaims, error) {
	parsed, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidJWT
		}
		return signer.secret, nil
	}, jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || parsed == nil || !parsed.Valid {
		return JWTClaims{}, ErrInvalidJWT
	}
	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return JWTClaims{}, ErrInvalidJWT
	}
	if signer.issuer != "" {
		if issuer, _ := mapClaims["iss"].(string); issuer != signer.issuer {
			return JWTClaims{}, ErrInvalidJWT
		}
	}
	return mapClaimsToJWTClaims(mapClaims)
}

func mapClaimsToJWTClaims(mapClaims jwt.MapClaims) (JWTClaims, error) {
	subject, _ := mapClaims["sub"].(string)
	role, _ := mapClaims["role"].(string)
	jti, _ := mapClaims["jti"].(string)
	issuedAtUnix, _ := mapClaims["iat"].(float64)
	expiresAtUnix, _ := mapClaims["exp"].(float64)
	if subject == "" || role == "" {
		return JWTClaims{}, ErrInvalidJWT
	}
	return JWTClaims{
		Subject:   subject,
		Role:      role,
		JTI:       jti,
		IssuedAt:  time.Unix(int64(issuedAtUnix), 0).UTC(),
		ExpiresAt: time.Unix(int64(expiresAtUnix), 0).UTC(),
	}, nil
}
