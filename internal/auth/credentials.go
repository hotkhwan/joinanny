package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// BinanceKeys is a user's plaintext Binance API credentials. It exists only in
// memory; at rest the secrets live as Sealed values inside BinanceCredential.
type BinanceKeys struct {
	APIKey    string
	APISecret string
	Testnet   bool
}

// BinanceCredential is the at-rest record for one user. The API key and secret
// are always encrypted here, never stored in plaintext.
type BinanceCredential struct {
	UserID    int64  `bson:"user_id" json:"user_id"`
	APIKey    Sealed `bson:"api_key" json:"api_key"`
	APISecret Sealed `bson:"api_secret" json:"api_secret"`
	Testnet   bool   `bson:"testnet" json:"testnet"`
}

// ErrNoCredential is returned when a user has no stored Binance credential.
var ErrNoCredential = errors.New("auth: no binance credential for user")

// CredentialRepository persists encrypted credentials. Implementations (e.g.
// MongoDB) only ever see Sealed values, never plaintext secrets. Find must
// return ErrNoCredential when the user has no record.
type CredentialRepository interface {
	Save(ctx context.Context, cred BinanceCredential) error
	Find(ctx context.Context, userID int64) (BinanceCredential, error)
	Remove(ctx context.Context, userID int64) error
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

// Store seals the user's keys and persists them, replacing any existing record.
func (s *CredentialService) Store(ctx context.Context, userID int64, keys BinanceKeys) error {
	if userID <= 0 {
		return errors.New("auth: user id must be positive")
	}
	if strings.TrimSpace(keys.APIKey) == "" || strings.TrimSpace(keys.APISecret) == "" {
		return errors.New("auth: api key and secret are required")
	}

	sealedKey, err := s.keyring.EncryptString(keys.APIKey)
	if err != nil {
		return fmt.Errorf("auth: seal api key: %w", err)
	}
	sealedSecret, err := s.keyring.EncryptString(keys.APISecret)
	if err != nil {
		return fmt.Errorf("auth: seal api secret: %w", err)
	}

	return s.repo.Save(ctx, BinanceCredential{
		UserID:    userID,
		APIKey:    sealedKey,
		APISecret: sealedSecret,
		Testnet:   keys.Testnet,
	})
}

// Load fetches and decrypts a user's keys. It returns ErrNoCredential if none
// are stored.
func (s *CredentialService) Load(ctx context.Context, userID int64) (BinanceKeys, error) {
	cred, err := s.repo.Find(ctx, userID)
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

// Delete removes a user's stored credential.
func (s *CredentialService) Delete(ctx context.Context, userID int64) error {
	return s.repo.Remove(ctx, userID)
}
