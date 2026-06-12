package signals

import (
	"context"
	"time"
)

type SignalStatus string

const (
	SignalStatusAccepted             SignalStatus = "accepted"
	SignalStatusHeld                 SignalStatus = "held"
	SignalStatusRejected             SignalStatus = "rejected"
	SignalStatusConfirmationRequired SignalStatus = "confirmation_required"
	SignalStatusExecuted             SignalStatus = "executed"
	SignalStatusFailed               SignalStatus = "failed"
)

type SignalRecord struct {
	Signal         MarketSignal
	Decision       Decision
	Status         SignalStatus
	Message        string
	ConfirmationID string
	ExecutionMode  string
	ClientOrderID  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SignalStore interface {
	PutSignalRecord(ctx context.Context, record SignalRecord) error
}

type NoopSignalStore struct{}

func (NoopSignalStore) PutSignalRecord(ctx context.Context, record SignalRecord) error {
	return nil
}
