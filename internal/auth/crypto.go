// Package auth holds per-user trading credentials and the encryption that keeps
// them safe at rest. Binance API keys are sealed with AES-256-GCM before they
// reach storage, so a database dump never exposes a usable secret.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// KeySize is the required master key length. AES-256 needs a 32-byte key.
const KeySize = 32

// Sealed is an encrypted value at rest. It records which master key (KeyID)
// produced it so keys can be rotated without re-encrypting existing data: new
// values use the active key while old values still decrypt via their KeyID.
type Sealed struct {
	KeyID      string `bson:"key_id" json:"key_id"`
	Nonce      []byte `bson:"nonce" json:"nonce"`
	Ciphertext []byte `bson:"ciphertext" json:"ciphertext"`
}

// IsZero reports whether s holds no encrypted data.
func (s Sealed) IsZero() bool {
	return s.KeyID == "" && len(s.Nonce) == 0 && len(s.Ciphertext) == 0
}

// Keyring encrypts with one active master key and decrypts with any key it
// knows, which is what makes key rotation possible: add a new key, mark it
// active, and previously sealed values still open via their recorded KeyID.
type Keyring struct {
	ciphers  map[string]cipher.AEAD
	activeID string
}

// NewKeyring builds a keyring from keyID -> master key pairs, encrypting new
// values with activeID. Every master key must be exactly KeySize bytes.
func NewKeyring(keys map[string][]byte, activeID string) (*Keyring, error) {
	if len(keys) == 0 {
		return nil, errors.New("auth: at least one master key is required")
	}
	if _, ok := keys[activeID]; !ok {
		return nil, fmt.Errorf("auth: active key id %q is not in the keyring", activeID)
	}

	ciphers := make(map[string]cipher.AEAD, len(keys))
	for id, key := range keys {
		if len(key) != KeySize {
			return nil, fmt.Errorf("auth: master key %q must be %d bytes, got %d", id, KeySize, len(key))
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("auth: new cipher for key %q: %w", id, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("auth: new gcm for key %q: %w", id, err)
		}
		ciphers[id] = aead
	}

	return &Keyring{ciphers: ciphers, activeID: activeID}, nil
}

// Encrypt seals plaintext with the active master key under a fresh random nonce,
// so encrypting the same value twice yields different ciphertext.
func (k *Keyring) Encrypt(plaintext []byte) (Sealed, error) {
	aead := k.ciphers[k.activeID]

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Sealed{}, fmt.Errorf("auth: read nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	return Sealed{KeyID: k.activeID, Nonce: nonce, Ciphertext: ciphertext}, nil
}

// Decrypt opens a sealed value using the master key named by s.KeyID. It fails
// if the key is unknown or the ciphertext has been tampered with (GCM auth tag).
func (k *Keyring) Decrypt(s Sealed) ([]byte, error) {
	aead, ok := k.ciphers[s.KeyID]
	if !ok {
		return nil, fmt.Errorf("auth: unknown key id %q", s.KeyID)
	}
	if len(s.Nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("auth: nonce size %d, want %d", len(s.Nonce), aead.NonceSize())
	}

	plaintext, err := aead.Open(nil, s.Nonce, s.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: decrypt: %w", err)
	}
	return plaintext, nil
}

// EncryptString is a convenience wrapper around Encrypt for string secrets.
func (k *Keyring) EncryptString(s string) (Sealed, error) {
	return k.Encrypt([]byte(s))
}

// DecryptString is a convenience wrapper around Decrypt for string secrets.
func (k *Keyring) DecryptString(s Sealed) (string, error) {
	plaintext, err := k.Decrypt(s)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
