package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// memRepo is an in-memory CredentialRepository for network-free tests. It also
// lets tests inspect exactly what would be persisted at rest.
type memRepo struct {
	saved map[int64]BinanceCredential
}

func newMemRepo() *memRepo { return &memRepo{saved: map[int64]BinanceCredential{}} }

func (r *memRepo) Save(_ context.Context, cred BinanceCredential) error {
	r.saved[cred.UserID] = cred
	return nil
}

func (r *memRepo) Find(_ context.Context, userID int64) (BinanceCredential, error) {
	cred, ok := r.saved[userID]
	if !ok {
		return BinanceCredential{}, ErrNoCredential
	}
	return cred, nil
}

func (r *memRepo) Remove(_ context.Context, userID int64) error {
	delete(r.saved, userID)
	return nil
}

func newTestService(t *testing.T) (*CredentialService, *memRepo) {
	t.Helper()
	repo := newMemRepo()
	svc, err := NewCredentialService(newTestKeyring(t), repo)
	if err != nil {
		t.Fatalf("NewCredentialService: %v", err)
	}
	return svc, repo
}

func TestCredentialStoreLoadRoundTrip(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	want := BinanceKeys{APIKey: "pubkey-123", APISecret: "secret-abc", Testnet: true}

	if err := svc.Store(ctx, 42, want); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := svc.Load(ctx, 42)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("loaded = %+v, want %+v", got, want)
	}
}

func TestCredentialStoredEncryptedAtRest(t *testing.T) {
	svc, repo := newTestService(t)
	keys := BinanceKeys{APIKey: "pubkey-plain", APISecret: "secret-plain"}

	if err := svc.Store(context.Background(), 7, keys); err != nil {
		t.Fatalf("Store: %v", err)
	}

	cred := repo.saved[7]
	if cred.APIKey.IsZero() || cred.APISecret.IsZero() {
		t.Fatal("credential stored without sealed key material")
	}
	if strings.Contains(string(cred.APIKey.Ciphertext), "pubkey-plain") {
		t.Fatal("api key persisted in plaintext")
	}
	if strings.Contains(string(cred.APISecret.Ciphertext), "secret-plain") {
		t.Fatal("api secret persisted in plaintext")
	}
}

func TestCredentialStoreValidatesInput(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.Store(ctx, 0, BinanceKeys{APIKey: "k", APISecret: "s"}); err == nil {
		t.Fatal("Store accepted a non-positive user id")
	}
	if err := svc.Store(ctx, 1, BinanceKeys{APIKey: "  ", APISecret: "s"}); err == nil {
		t.Fatal("Store accepted a blank api key")
	}
}

func TestCredentialLoadMissingReturnsSentinel(t *testing.T) {
	svc, _ := newTestService(t)

	_, err := svc.Load(context.Background(), 999)
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load error = %v, want ErrNoCredential", err)
	}
}

func TestCredentialDelete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.Store(ctx, 5, BinanceKeys{APIKey: "k", APISecret: "s"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := svc.Delete(ctx, 5); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Load(ctx, 5); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("after delete Load error = %v, want ErrNoCredential", err)
	}
}

func TestNewCredentialServiceRequiresDeps(t *testing.T) {
	if _, err := NewCredentialService(nil, newMemRepo()); err == nil {
		t.Fatal("expected error for nil keyring")
	}
	if _, err := NewCredentialService(newTestKeyring(t), nil); err == nil {
		t.Fatal("expected error for nil repository")
	}
}
