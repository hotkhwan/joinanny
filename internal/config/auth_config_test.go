package config

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func base64Key(n int) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'k'}, n))
}

func TestLoadCredentialEncryptionKeyDisabledByDefault(t *testing.T) {
	cfg, err := LoadFromLookup(testLookup(nil))
	if err != nil {
		t.Fatalf("LoadFromLookup returned error: %v", err)
	}
	if cfg.Auth.Enabled {
		t.Fatal("Auth.Enabled = true with no key set, want false")
	}
}

func TestLoadCredentialEncryptionKeyAccepts32Bytes(t *testing.T) {
	cfg, err := LoadFromLookup(testLookup(map[string]string{
		"CREDENTIAL_ENCRYPTION_KEY": base64Key(32),
	}))
	if err != nil {
		t.Fatalf("LoadFromLookup returned error: %v", err)
	}
	if !cfg.Auth.Enabled {
		t.Fatal("Auth.Enabled = false, want true")
	}
	if len(cfg.Auth.EncryptionKey) != 32 {
		t.Fatalf("EncryptionKey length = %d, want 32", len(cfg.Auth.EncryptionKey))
	}
	if cfg.Auth.EncryptionKeyID != "v1" {
		t.Fatalf("EncryptionKeyID = %q, want default v1", cfg.Auth.EncryptionKeyID)
	}
}

func TestLoadCredentialEncryptionKeyHonorsKeyID(t *testing.T) {
	cfg, err := LoadFromLookup(testLookup(map[string]string{
		"CREDENTIAL_ENCRYPTION_KEY":    base64Key(32),
		"CREDENTIAL_ENCRYPTION_KEY_ID": "2026-06",
	}))
	if err != nil {
		t.Fatalf("LoadFromLookup returned error: %v", err)
	}
	if cfg.Auth.EncryptionKeyID != "2026-06" {
		t.Fatalf("EncryptionKeyID = %q, want 2026-06", cfg.Auth.EncryptionKeyID)
	}
}

func TestLoadCredentialEncryptionKeyRejectsWrongLength(t *testing.T) {
	_, err := LoadFromLookup(testLookup(map[string]string{
		"CREDENTIAL_ENCRYPTION_KEY": base64Key(16),
	}))
	if err == nil {
		t.Fatal("LoadFromLookup returned nil error for a 16-byte key, want validation error")
	}
}

func TestLoadCredentialEncryptionKeyRejectsInvalidBase64(t *testing.T) {
	_, err := LoadFromLookup(testLookup(map[string]string{
		"CREDENTIAL_ENCRYPTION_KEY": "not-valid-base64!!!",
	}))
	if err == nil {
		t.Fatal("LoadFromLookup returned nil error for invalid base64, want validation error")
	}
}
