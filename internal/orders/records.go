package orders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"bottrade/internal/domain"
)

type IntentStatus string

const (
	IntentStatusAwaitingConfirmation IntentStatus = "awaiting_confirmation"
	IntentStatusExecuting            IntentStatus = "executing"
	IntentStatusExecuted             IntentStatus = "executed"
	IntentStatusCancelled            IntentStatus = "cancelled"
	IntentStatusExpired              IntentStatus = "expired"
	IntentStatusFailed               IntentStatus = "failed"
)

type IntentRecord struct {
	ID             string
	UserID         int64
	Intent         domain.Intent
	IntentHash     string
	CorrelationID  string
	ConfirmationID string
	Status         IntentStatus
	ErrorMessage   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type IntentStore interface {
	PutIntentRecord(ctx context.Context, record IntentRecord) error
	UpdateIntentStatus(ctx context.Context, id string, status IntentStatus, errorMessage string, updatedAt time.Time) error
}

type NoopIntentStore struct{}

func (NoopIntentStore) PutIntentRecord(ctx context.Context, record IntentRecord) error {
	return nil
}

func (NoopIntentStore) UpdateIntentStatus(ctx context.Context, id string, status IntentStatus, errorMessage string, updatedAt time.Time) error {
	return nil
}

func IntentHash(intent domain.Intent) (string, error) {
	payload, err := json.Marshal(intent)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}
