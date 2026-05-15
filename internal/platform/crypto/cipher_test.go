package crypto

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestAESGCMCipherEncryptDecryptRoundTrip(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	cipher, err := NewAESGCMCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}

	cases := []struct {
		name      string
		plaintext []byte
	}{
		{name: "empty", plaintext: []byte{}},
		{name: "ascii", plaintext: []byte("dynatrace-api-token-123")},
		{name: "utf8", plaintext: []byte("análise crítica 🚨")},
		{name: "binary", plaintext: bytes.Repeat([]byte{0xff, 0x00, 0x42}, 64)},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			sealed, err := cipher.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			if !strings.HasPrefix(sealed, "v1:") {
				t.Fatalf("ciphertext is missing the v1 prefix: %q", sealed)
			}
			opened, err := cipher.Decrypt(sealed)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(opened, tt.plaintext) {
				t.Fatalf("round trip mismatch: got %q want %q", opened, tt.plaintext)
			}
		})
	}
}

func TestAESGCMCipherEncryptProducesFreshNonce(t *testing.T) {
	key, _ := GenerateKey()
	cipher, _ := NewAESGCMCipher(key)
	plaintext := []byte("repeat")
	first, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt first: %v", err)
	}
	second, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt second: %v", err)
	}
	if first == second {
		t.Fatalf("encryption of identical plaintext produced identical ciphertext: %q", first)
	}
}

func TestNewAESGCMCipherRejectsWrongKeyLength(t *testing.T) {
	cases := [][]byte{nil, {}, make([]byte, 16), make([]byte, 31), make([]byte, 64)}
	for _, key := range cases {
		if _, err := NewAESGCMCipher(key); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("expected ErrInvalidKey for key length %d, got %v", len(key), err)
		}
	}
}

func TestAESGCMCipherDecryptRejectsCorruption(t *testing.T) {
	key, _ := GenerateKey()
	cipher, _ := NewAESGCMCipher(key)
	sealed, err := cipher.Encrypt([]byte("ok"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	cases := []struct {
		name       string
		ciphertext string
	}{
		{name: "empty", ciphertext: ""},
		{name: "no_version", ciphertext: "abc:def"},
		{name: "wrong_version", ciphertext: "v2:" + strings.TrimPrefix(sealed, "v1:")},
		{name: "bad_nonce_b64", ciphertext: "v1:!!!:" + base64.StdEncoding.EncodeToString([]byte("ok"))},
		{name: "tampered_payload", ciphertext: sealed + "AA"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := cipher.Decrypt(tt.ciphertext); !errors.Is(err, ErrInvalidCiphertext) {
				t.Fatalf("expected ErrInvalidCiphertext, got %v", err)
			}
		})
	}
}

func TestAESGCMCipherDecryptRejectsForeignKey(t *testing.T) {
	firstKey, _ := GenerateKey()
	secondKey, _ := GenerateKey()
	producer, _ := NewAESGCMCipher(firstKey)
	consumer, _ := NewAESGCMCipher(secondKey)
	sealed, err := producer.Encrypt([]byte("hello"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := consumer.Decrypt(sealed); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("expected ErrInvalidCiphertext when decrypting with foreign key, got %v", err)
	}
}

func TestLoadKeyAcceptsMultipleEncodings(t *testing.T) {
	raw := bytes.Repeat([]byte{0xab, 0xcd}, 16)
	cases := []struct {
		name    string
		encoded string
	}{
		{name: "hex", encoded: hex.EncodeToString(raw)},
		{name: "std_b64", encoded: base64.StdEncoding.EncodeToString(raw)},
		{name: "raw_std_b64", encoded: base64.RawStdEncoding.EncodeToString(raw)},
		{name: "url_b64", encoded: base64.URLEncoding.EncodeToString(raw)},
		{name: "raw_url_b64", encoded: base64.RawURLEncoding.EncodeToString(raw)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := LoadKey(tt.encoded)
			if err != nil {
				t.Fatalf("load key: %v", err)
			}
			if !bytes.Equal(decoded, raw) {
				t.Fatalf("decoded key mismatch: got %x want %x", decoded, raw)
			}
		})
	}
}

func TestLoadKeyRejectsInvalidInput(t *testing.T) {
	cases := []string{"", "   ", "too-short", hex.EncodeToString(make([]byte, 16))}
	for _, encoded := range cases {
		if _, err := LoadKey(encoded); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("expected ErrInvalidKey for %q, got %v", encoded, err)
		}
	}
}
