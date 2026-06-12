package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bottrade/internal/signals"
)

type signalDoc struct {
	Source         string               `bson:"source"`
	Symbol         string               `bson:"symbol"`
	Interval       string               `bson:"interval,omitempty"`
	Strategy       string               `bson:"strategy,omitempty"`
	Status         signals.SignalStatus `bson:"status"`
	SignalJSON     string               `bson:"signal_json"`
	DecisionJSON   string               `bson:"decision_json"`
	Message        string               `bson:"message,omitempty"`
	ConfirmationID string               `bson:"confirmation_id,omitempty"`
	ExecutionMode  string               `bson:"execution_mode,omitempty"`
	ClientOrderID  string               `bson:"client_order_id,omitempty"`
	CreatedAt      time.Time            `bson:"created_at"`
	UpdatedAt      time.Time            `bson:"updated_at"`
}

func (s *Store) PutSignalRecord(ctx context.Context, record signals.SignalRecord) error {
	doc, err := newSignalDoc(record)
	if err != nil {
		return err
	}
	if _, err := s.signals.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("insert signal record: %w", err)
	}
	return nil
}

func newSignalDoc(record signals.SignalRecord) (signalDoc, error) {
	signalJSON, err := json.Marshal(record.Signal)
	if err != nil {
		return signalDoc{}, fmt.Errorf("marshal signal: %w", err)
	}
	decisionJSON, err := json.Marshal(record.Decision)
	if err != nil {
		return signalDoc{}, fmt.Errorf("marshal signal decision: %w", err)
	}

	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = record.Signal.ReceivedAt
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	return signalDoc{
		Source:         record.Signal.Source,
		Symbol:         record.Signal.Symbol,
		Interval:       record.Signal.Interval,
		Strategy:       record.Signal.Strategy,
		Status:         record.Status,
		SignalJSON:     string(signalJSON),
		DecisionJSON:   string(decisionJSON),
		Message:        record.Message,
		ConfirmationID: record.ConfirmationID,
		ExecutionMode:  record.ExecutionMode,
		ClientOrderID:  record.ClientOrderID,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}
