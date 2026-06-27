package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// DefaultProfile is the profile name used when a caller does not specify one.
const DefaultProfile = "default"

// BinanceKeys is a user's plaintext Binance API credentials. It exists only in
// memory; at rest the secrets live as Sealed values inside BinanceCredential.
type BinanceKeys struct {
	APIKey    string
	APISecret string
	Testnet   bool
}

// BinanceCredential is the at-rest record for one of a user's key profiles. The
// API key and secret are always encrypted here, never stored in plaintext. A
// user may have several profiles (e.g. "testnet" and "live"); exactly one is
// Active and is the one trades run on.
type BinanceCredential struct {
	UserID    string `bson:"user_id" json:"user_id"`
	Profile   string `bson:"profile" json:"profile"`
	APIKey    Sealed `bson:"api_key" json:"api_key"`
	APISecret Sealed `bson:"api_secret" json:"api_secret"`
	Testnet   bool   `bson:"testnet" json:"testnet"`
	Active    bool   `bson:"active" json:"active"`
}

// ProfileInfo is the non-secret summary of one key profile, safe to return to
// the dashboard.
type ProfileInfo struct {
	Profile    string `json:"profile"`
	Testnet    bool   `json:"testnet"`
	Active     bool   `json:"active"`
	APIKeyTail string `json:"api_key_tail"`
}

// ErrNoCredential is returned when a user has no active Binance credential.
var ErrNoCredential = errors.New("auth: no binance credential for user")

// CredentialRepository persists encrypted credential profiles. Implementations
// only ever see Sealed values, never plaintext secrets. Records are keyed by
// (UserID, Profile). FindActive must return ErrNoCredential when the user has no
// active profile.
type CredentialRepository interface {
	Save(ctx context.Context, cred BinanceCredential) error
	List(ctx context.Context, userID string) ([]BinanceCredential, error)
	FindActive(ctx context.Context, userID string) (BinanceCredential, error)
	Remove(ctx context.Context, userID, profile string) error
	SetActive(ctx context.Context, userID, profile string) error
}

// CredentialService encrypts credentials before they reach the repository and
// decrypts them on the way out, so plaintext Binance secrets never touch
// storage. It is the boundary the rest of the bot uses for per-user keys.
type CredentialService struct {
	keyring *Keyring
	repo    CredentialRepository
}

// NewCredentialService wires a keyring and repository together.
func NewCredentialService(keyring *Keyring, repo CredentialRepository) (*CredentialService, error) {
	if keyring == nil {
		return nil, errors.New("auth: keyring is required")
	}
	if repo == nil {
		return nil, errors.New("auth: credential repository is required")
	}
	return &CredentialService{keyring: keyring, repo: repo}, nil
}

// StoreProfile seals the user's keys and persists them under the named profile,
// replacing any existing profile of that name. When it is the user's first
// profile it becomes active automatically.
func (s *CredentialService) StoreProfile(ctx context.Context, userID, profile string, keys BinanceKeys) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("auth: user id is required")
	}
	profile = normalizeProfile(profile)
	if strings.TrimSpace(keys.APIKey) == "" || strings.TrimSpace(keys.APISecret) == "" {
		return errors.New("auth: api key and secret are required")
	}

	existing, err := s.repo.List(ctx, userID)
	if err != nil {
		return err
	}
	active := len(existing) == 0 // first profile becomes active
	for _, cred := range existing {
		if cred.Profile == profile && cred.Active {
			active = true // keep an updated profile active
		}
	}

	sealedKey, err := s.keyring.EncryptString(keys.APIKey)
	if err != nil {
		return fmt.Errorf("auth: seal api key: %w", err)
	}
	sealedSecret, err := s.keyring.EncryptString(keys.APISecret)
	if err != nil {
		return fmt.Errorf("auth: seal api secret: %w", err)
	}

	if err := s.repo.Save(ctx, BinanceCredential{
		UserID: userID, Profile: profile,
		APIKey: sealedKey, APISecret: sealedSecret,
		Testnet: keys.Testnet, Active: active,
	}); err != nil {
		return err
	}
	if active {
		return s.repo.SetActive(ctx, userID, profile)
	}
	return nil
}

// Store stores a single default profile (compatibility shim).
func (s *CredentialService) Store(ctx context.Context, userID string, keys BinanceKeys) error {
	return s.StoreProfile(ctx, userID, DefaultProfile, keys)
}

// Load fetches and decrypts the user's active profile keys. It returns
// ErrNoCredential if none are active.
func (s *CredentialService) Load(ctx context.Context, userID string) (BinanceKeys, error) {
	cred, err := s.repo.FindActive(ctx, userID)
	if err != nil {
		return BinanceKeys{}, err
	}
	apiKey, err := s.keyring.DecryptString(cred.APIKey)
	if err != nil {
		return BinanceKeys{}, fmt.Errorf("auth: open api key: %w", err)
	}
	apiSecret, err := s.keyring.DecryptString(cred.APISecret)
	if err != nil {
		return BinanceKeys{}, fmt.Errorf("auth: open api secret: %w", err)
	}
	return BinanceKeys{APIKey: apiKey, APISecret: apiSecret, Testnet: cred.Testnet}, nil
}

// Profiles returns the non-secret summary of every profile for the user, with
// the masked tail of each API key.
func (s *CredentialService) Profiles(ctx context.Context, userID string) ([]ProfileInfo, error) {
	creds, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	infos := make([]ProfileInfo, 0, len(creds))
	for _, cred := range creds {
		tail := ""
		if apiKey, derr := s.keyring.DecryptString(cred.APIKey); derr == nil {
			tail = maskKeyTail(apiKey)
		}
		infos = append(infos, ProfileInfo{
			Profile: cred.Profile, Testnet: cred.Testnet, Active: cred.Active, APIKeyTail: tail,
		})
	}
	return infos, nil
}

// SetActive makes the named profile the one trades run on.
func (s *CredentialService) SetActive(ctx context.Context, userID, profile string) error {
	return s.repo.SetActive(ctx, userID, normalizeProfile(profile))
}

// DeleteProfile removes one profile. If it was the active one and other profiles
// remain, the first remaining profile is promoted to active so the user is never
// left with profiles but none active.
func (s *CredentialService) DeleteProfile(ctx context.Context, userID, profile string) error {
	profile = normalizeProfile(profile)
	if err := s.repo.Remove(ctx, userID, profile); err != nil {
		return err
	}
	remaining, err := s.repo.List(ctx, userID)
	if err != nil {
		return err
	}
	if len(remaining) == 0 {
		return nil
	}
	for _, cred := range remaining {
		if cred.Active {
			return nil // an active profile still exists
		}
	}
	return s.repo.SetActive(ctx, userID, remaining[0].Profile)
}

// Delete removes all of the user's profiles (compatibility shim).
func (s *CredentialService) Delete(ctx context.Context, userID string) error {
	creds, err := s.repo.List(ctx, userID)
	if err != nil {
		return err
	}
	for _, cred := range creds {
		if err := s.repo.Remove(ctx, userID, cred.Profile); err != nil {
			return err
		}
	}
	return nil
}

func normalizeProfile(profile string) string {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		return DefaultProfile
	}
	return profile
}

func maskKeyTail(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if len(apiKey) <= 4 {
		return "set"
	}
	return "…" + apiKey[len(apiKey)-4:]
}
