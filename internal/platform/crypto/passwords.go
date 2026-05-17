package crypto

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// DefaultPasswordCost is the bcrypt cost applied when callers do not supply
// one. The value balances login latency against modern brute-force resistance.
const DefaultPasswordCost = 12

// ErrPasswordMismatch is returned when the supplied password does not match
// the stored hash.
var ErrPasswordMismatch = errors.New("password mismatch")

// HashPassword returns the bcrypt hash for the supplied password using cost.
//
// When cost is zero or negative DefaultPasswordCost is used.
func HashPassword(password string, cost int) (string, error) {
	if cost <= 0 {
		cost = DefaultPasswordCost
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hashed), nil
}

// VerifyPassword reports whether the supplied password matches the stored
// bcrypt hash.
//
// A mismatch surfaces as ErrPasswordMismatch; any other failure (malformed
// hash, internal bcrypt error) is returned wrapped.
func VerifyPassword(hash string, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrPasswordMismatch
	}
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	return nil
}

// BcryptPasswordHasher is the bcrypt-backed adapter that satisfies
// ports.PasswordHasher.
//
// Cost overrides the bcrypt cost; values <= 0 fall back to
// DefaultPasswordCost. Tests can use a low cost to keep runs fast.
type BcryptPasswordHasher struct {
	cost int
}

// NewBcryptPasswordHasher returns a hasher that applies the supplied
// bcrypt cost. A cost <= 0 falls back to DefaultPasswordCost.
func NewBcryptPasswordHasher(cost int) *BcryptPasswordHasher {
	return &BcryptPasswordHasher{cost: cost}
}

// Hash implements ports.PasswordHasher.
func (hasher *BcryptPasswordHasher) Hash(password string) (string, error) {
	return HashPassword(password, hasher.cost)
}

// Verify implements ports.PasswordHasher.
func (hasher *BcryptPasswordHasher) Verify(hash string, password string) error {
	return VerifyPassword(hash, password)
}
