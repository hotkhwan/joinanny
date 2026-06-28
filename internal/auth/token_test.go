package auth

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func tokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	tk, err := NewTokenizer(bytes.Repeat([]byte("s"), MinSecretSize), time.Hour)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}
	return tk
}

func TestNewTokenizerRequiresStrongSecret(t *testing.T) {
	if _, err := NewTokenizer([]byte("short"), time.Hour); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestIssueVerifyRoundTrip(t *testing.T) {
	tk := tokenizer(t)
	token, err := tk.Issue("u1", "alice", "trader")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := tk.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "u1" || claims.Username != "alice" || claims.Role != "trader" {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestVerifyRejectsTamperedAndWrongSecret(t *testing.T) {
	tk := tokenizer(t)
	token, _ := tk.Issue("u1", "alice", "trader")

	// Tamper the payload segment.
	parts := strings.Split(token, ".")
	parts[1] = parts[1][:len(parts[1])-1] + "A"
	if _, err := tk.Verify(strings.Join(parts, ".")); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("tampered err = %v, want ErrTokenInvalid", err)
	}

	// A different secret must reject the signature.
	other, _ := NewTokenizer(bytes.Repeat([]byte("x"), MinSecretSize), time.Hour)
	if _, err := other.Verify(token); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("wrong-secret err = %v, want ErrTokenInvalid", err)
	}

	if _, err := tk.Verify("not.a.jwt.token"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("malformed err = %v, want ErrTokenInvalid", err)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	tk := tokenizer(t)
	base := time.Unix(1_700_000_000, 0)
	tk.now = func() time.Time { return base }
	token, _ := tk.Issue("u1", "alice", "trader")

	tk.now = func() time.Time { return base.Add(2 * time.Hour) }
	if _, err := tk.Verify(token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expired err = %v, want ErrTokenExpired", err)
	}
}
