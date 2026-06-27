package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// memRepo is an in-memory CredentialRepository for network-free tests, keyed by
// (user, profile). It also lets tests inspect what would be persisted at rest.
type memRepo struct {
	saved map[string][]BinanceCredential // userID -> profiles
}

func newMemRepo() *memRepo { return &memRepo{saved: map[string][]BinanceCredential{}} }

func (r *memRepo) Save(_ context.Context, cred BinanceCredential) error {
	list := r.saved[cred.UserID]
	for i := range list {
		if list[i].Profile == cred.Profile {
			list[i] = cred
			r.saved[cred.UserID] = list
			return nil
		}
	}
	r.saved[cred.UserID] = append(list, cred)
	return nil
}

func (r *memRepo) List(_ context.Context, userID string) ([]BinanceCredential, error) {
	return r.saved[userID], nil
}

func (r *memRepo) FindActive(_ context.Context, userID string) (BinanceCredential, error) {
	for _, c := range r.saved[userID] {
		if c.Active {
			return c, nil
		}
	}
	return BinanceCredential{}, ErrNoCredential
}

func (r *memRepo) Remove(_ context.Context, userID, profile string) error {
	list := r.saved[userID]
	out := list[:0]
	for _, c := range list {
		if c.Profile != profile {
			out = append(out, c)
		}
	}
	r.saved[userID] = out
	return nil
}

func (r *memRepo) SetActive(_ context.Context, userID, profile string) error {
	list := r.saved[userID]
	for i := range list {
		list[i].Active = list[i].Profile == profile
	}
	r.saved[userID] = list
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

	if err := svc.Store(ctx, "42", want); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := svc.Load(ctx, "42")
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
	if err := svc.Store(context.Background(), "7", keys); err != nil {
		t.Fatalf("Store: %v", err)
	}

	cred := repo.saved["7"][0]
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

func TestCredentialMultiProfile(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// First profile becomes active automatically.
	if err := svc.StoreProfile(ctx, "9", "testnet", BinanceKeys{APIKey: "tk-aaaa", APISecret: "ts", Testnet: true}); err != nil {
		t.Fatalf("store testnet: %v", err)
	}
	if err := svc.StoreProfile(ctx, "9", "live", BinanceKeys{APIKey: "lk-bbbb", APISecret: "ls"}); err != nil {
		t.Fatalf("store live: %v", err)
	}

	profiles, err := svc.Profiles(ctx, "9")
	if err != nil || len(profiles) != 2 {
		t.Fatalf("profiles = %+v (err %v), want 2", profiles, err)
	}
	// testnet is active and masks its key tail.
	for _, p := range profiles {
		if p.Profile == "testnet" {
			if !p.Active || p.APIKeyTail != "…aaaa" {
				t.Fatalf("testnet profile = %+v, want active with masked tail", p)
			}
		}
		if p.Profile == "live" && p.Active {
			t.Fatal("live should not be active yet")
		}
	}

	// Active load returns the testnet key.
	if got, _ := svc.Load(ctx, "9"); got.APIKey != "tk-aaaa" {
		t.Fatalf("active key = %q, want tk-aaaa", got.APIKey)
	}

	// Switch active to live.
	if err := svc.SetActive(ctx, "9", "live"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if got, _ := svc.Load(ctx, "9"); got.APIKey != "lk-bbbb" {
		t.Fatalf("after switch active key = %q, want lk-bbbb", got.APIKey)
	}

	// Deleting the active profile promotes the remaining one.
	if err := svc.DeleteProfile(ctx, "9", "live"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if got, _ := svc.Load(ctx, "9"); got.APIKey != "tk-aaaa" {
		t.Fatalf("after delete active key = %q, want tk-aaaa (promoted)", got.APIKey)
	}
}

func TestCredentialStoreValidatesInput(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if err := svc.Store(ctx, "", BinanceKeys{APIKey: "k", APISecret: "s"}); err == nil {
		t.Fatal("Store accepted a blank user id")
	}
	if err := svc.Store(ctx, "1", BinanceKeys{APIKey: "  ", APISecret: "s"}); err == nil {
		t.Fatal("Store accepted a blank api key")
	}
}

func TestCredentialLoadMissingReturnsSentinel(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.Load(context.Background(), "999"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("Load error = %v, want ErrNoCredential", err)
	}
}

func TestCredentialDelete(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if err := svc.Store(ctx, "5", BinanceKeys{APIKey: "k", APISecret: "s"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := svc.Delete(ctx, "5"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Load(ctx, "5"); !errors.Is(err, ErrNoCredential) {
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
