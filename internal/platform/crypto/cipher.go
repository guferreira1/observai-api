// Package crypto exposes symmetric encryption primitives used by ObservAI API
// to protect operator-supplied secrets at rest.
//
// All values are encrypted with AES-256-GCM and serialized as
// "v1:<nonce_b64>:<ciphertext_b64>". The version prefix lets future rotations
// migrate to alternative ciphers without breaking previously stored values.
package crypto

import (
	"crypto/aes"
	cryptocipher "crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// KeyLength is the AES-256 key length, in bytes.
const KeyLength = 32

const ciphertextVersion = "v1"

// ErrInvalidKey indicates the supplied encryption key is not 32 bytes long.
var ErrInvalidKey = errors.New("encryption key must be 32 bytes")

// ErrInvalidCiphertext indicates the supplied ciphertext could not be parsed,
// has an unsupported version prefix or failed authentication.
var ErrInvalidCiphertext = errors.New("invalid ciphertext")

// AESGCMCipher encrypts and decrypts byte payloads using AES-256-GCM.
type AESGCMCipher struct {
	aead cryptocipher.AEAD
}

// NewAESGCMCipher returns a cipher backed by the supplied 32-byte key.
func NewAESGCMCipher(key []byte) (*AESGCMCipher, error) {
	if len(key) != KeyLength {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes block cipher: %w", err)
	}
	aead, err := cryptocipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm aead: %w", err)
	}
	return &AESGCMCipher{aead: aead}, nil
}

// Encrypt seals plaintext with a fresh random nonce and returns the
// versioned base64-encoded payload.
func (cipher *AESGCMCipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, cipher.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	sealed := cipher.aead.Seal(nil, nonce, plaintext, nil)
	return strings.Join([]string{
		ciphertextVersion,
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(sealed),
	}, ":"), nil
}

// Decrypt parses the versioned payload and returns the original plaintext.
//
// Any parsing, decoding or authentication failure is collapsed into
// ErrInvalidCiphertext so callers cannot distinguish the failure mode and
// inadvertently leak oracle information.
func (cipher *AESGCMCipher) Decrypt(ciphertext string) ([]byte, error) {
	parts := strings.SplitN(ciphertext, ":", 3)
	if len(parts) != 3 || parts[0] != ciphertextVersion {
		return nil, ErrInvalidCiphertext
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(nonce) != cipher.aead.NonceSize() {
		return nil, ErrInvalidCiphertext
	}
	sealed, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	plaintext, err := cipher.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

// LoadKey decodes the supplied encoded key into raw bytes.
//
// Accepted encodings, in order of preference: 64-character hex, standard
// base64 (with or without padding) and URL-safe base64 (with or without
// padding). The decoded value must be exactly KeyLength bytes long.
func LoadKey(encoded string) ([]byte, error) {
	cleaned := strings.TrimSpace(encoded)
	if cleaned == "" {
		return nil, ErrInvalidKey
	}
	decoders := []func(string) ([]byte, error){
		hex.DecodeString,
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}
	for _, decode := range decoders {
		if decoded, err := decode(cleaned); err == nil && len(decoded) == KeyLength {
			return decoded, nil
		}
	}
	return nil, ErrInvalidKey
}

// GenerateKey returns a freshly generated 32-byte key suitable for local
// development. Production deployments must supply a key from a secret manager.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeyLength)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return key, nil
}
