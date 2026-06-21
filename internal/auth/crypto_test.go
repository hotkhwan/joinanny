package auth

import (
	"bytes"
	"strings"
	"testing"
)

func testKey(fill byte) []byte {
	return bytes.Repeat([]byte{fill}, KeySize)
}

func newTestKeyring(t *testing.T) *Keyring {
	t.Helper()
	kr, err := NewKeyring(map[string][]byte{"v1": testKey('a')}, "v1")
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	return kr
}

func TestNewKeyringValidation(t *testing.T) {
	cases := []struct {
		name     string
		keys     map[string][]byte
		activeID string
		wantErr  string
	}{
		{"empty", map[string][]byte{}, "v1", "at least one master key"},
		{"active missing", map[string][]byte{"v1": testKey('a')}, "v2", "active key id"},
		{"short key", map[string][]byte{"v1": bytes.Repeat([]byte{'a'}, 16)}, "v1", "must be 32 bytes"},
		{"ok", map[string][]byte{"v1": testKey('a')}, "v1", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewKeyring(tc.keys, tc.activeID)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	kr := newTestKeyring(t)
	const secret = "binance-api-secret-9f8b"

	sealed, err := kr.EncryptString(secret)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if sealed.KeyID != "v1" {
		t.Fatalf("KeyID = %q, want v1", sealed.KeyID)
	}
	if bytes.Contains(sealed.Ciphertext, []byte(secret)) {
		t.Fatal("ciphertext contains the plaintext secret")
	}

	got, err := kr.DecryptString(sealed)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}
	if got != secret {
		t.Fatalf("decrypted = %q, want %q", got, secret)
	}
}

func TestEncryptUsesFreshNonce(t *testing.T) {
	kr := newTestKeyring(t)

	a, err := kr.EncryptString("same")
	if err != nil {
		t.Fatalf("encrypt a: %v", err)
	}
	b, err := kr.EncryptString("same")
	if err != nil {
		t.Fatalf("encrypt b: %v", err)
	}
	if bytes.Equal(a.Nonce, b.Nonce) {
		t.Fatal("nonce reused across encryptions")
	}
	if bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Fatal("identical ciphertext for identical plaintext (deterministic encryption)")
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	kr := newTestKeyring(t)
	sealed, err := kr.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	sealed.Ciphertext[0] ^= 0xFF
	if _, err := kr.Decrypt(sealed); err == nil {
		t.Fatal("decrypt accepted tampered ciphertext")
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	enc := newTestKeyring(t) // key 'a'
	sealed, err := enc.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Same key id, different key material: GCM auth must reject it.
	dec, err := NewKeyring(map[string][]byte{"v1": testKey('z')}, "v1")
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	if _, err := dec.Decrypt(sealed); err == nil {
		t.Fatal("decrypt accepted a value sealed with a different key")
	}
}

func TestDecryptRejectsUnknownKeyID(t *testing.T) {
	kr := newTestKeyring(t)
	if _, err := kr.Decrypt(Sealed{KeyID: "v9", Nonce: make([]byte, 12), Ciphertext: []byte("x")}); err == nil {
		t.Fatal("decrypt accepted an unknown key id")
	}
}

func TestKeyRotationDecryptsOldRecords(t *testing.T) {
	// A value sealed under the old active key (v1)...
	old, err := NewKeyring(map[string][]byte{"v1": testKey('a')}, "v1")
	if err != nil {
		t.Fatalf("old keyring: %v", err)
	}
	sealed, err := old.EncryptString("legacy-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// ...still decrypts after rotating to v2 as active, and new values use v2.
	rotated, err := NewKeyring(map[string][]byte{"v1": testKey('a'), "v2": testKey('b')}, "v2")
	if err != nil {
		t.Fatalf("rotated keyring: %v", err)
	}

	got, err := rotated.DecryptString(sealed)
	if err != nil {
		t.Fatalf("decrypt legacy after rotation: %v", err)
	}
	if got != "legacy-secret" {
		t.Fatalf("decrypted = %q, want legacy-secret", got)
	}

	fresh, err := rotated.EncryptString("new-secret")
	if err != nil {
		t.Fatalf("encrypt new: %v", err)
	}
	if fresh.KeyID != "v2" {
		t.Fatalf("new value KeyID = %q, want v2 (active)", fresh.KeyID)
	}
}
