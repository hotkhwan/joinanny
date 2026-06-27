package mongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"bottrade/internal/auth"
)

// CredentialRepository persists per-user encrypted Binance credentials. It
// implements auth.CredentialRepository and only ever stores Sealed values.
type CredentialRepository struct {
	coll *mongodriver.Collection
}

// Credentials returns the MongoDB-backed credential repository.
func (s *Store) Credentials() *CredentialRepository {
	return &CredentialRepository{coll: s.credentials}
}

// Save upserts a user's sealed credential by user id.
func (r *CredentialRepository) Save(ctx context.Context, cred auth.BinanceCredential) error {
	if _, err := r.coll.ReplaceOne(ctx, bson.M{"user_id": cred.UserID}, cred, options.Replace().SetUpsert(true)); err != nil {
		return fmt.Errorf("save credential: %w", err)
	}
	return nil
}

// Find returns the user's sealed credential or auth.ErrNoCredential.
func (r *CredentialRepository) Find(ctx context.Context, userID string) (auth.BinanceCredential, error) {
	var cred auth.BinanceCredential
	err := r.coll.FindOne(ctx, bson.M{"user_id": userID}).Decode(&cred)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return auth.BinanceCredential{}, auth.ErrNoCredential
	}
	if err != nil {
		return auth.BinanceCredential{}, fmt.Errorf("find credential: %w", err)
	}
	return cred, nil
}

// Remove deletes a user's credential.
func (r *CredentialRepository) Remove(ctx context.Context, userID string) error {
	if _, err := r.coll.DeleteOne(ctx, bson.M{"user_id": userID}); err != nil {
		return fmt.Errorf("remove credential: %w", err)
	}
	return nil
}
