package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bottrade/internal/domain"
	"bottrade/internal/orders"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type orderIntentDoc struct {
	ID             string              `bson:"_id"`
	UserID         int64               `bson:"user_id"`
	IntentJSON     string              `bson:"intent_json"`
	IntentHash     string              `bson:"intent_hash"`
	IntentType     string              `bson:"intent_type"`
	Symbol         string              `bson:"symbol,omitempty"`
	PlanID         string              `bson:"plan_id,omitempty"`
	CorrelationID  string              `bson:"correlation_id"`
	ConfirmationID string              `bson:"confirmation_id"`
	Status         orders.IntentStatus `bson:"status"`
	ErrorMessage   string              `bson:"error_message,omitempty"`
	CreatedAt      time.Time           `bson:"created_at"`
	UpdatedAt      time.Time           `bson:"updated_at"`
}

func (s *Store) PutIntentRecord(ctx context.Context, record orders.IntentRecord) error {
	doc, err := newOrderIntentDoc(record)
	if err != nil {
		return err
	}
	if _, err := s.orderIntents.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("insert order intent: %w", err)
	}
	return nil
}

func (s *Store) UpdateIntentStatus(ctx context.Context, id string, status orders.IntentStatus, errorMessage string, updatedAt time.Time) error {
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "status", Value: status},
		{Key: "error_message", Value: errorMessage},
		{Key: "updated_at", Value: updatedAt},
	}}}
	_, err := s.orderIntents.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, update)
	if err != nil {
		return fmt.Errorf("update order intent status: %w", err)
	}
	return nil
}

func newOrderIntentDoc(record orders.IntentRecord) (orderIntentDoc, error) {
	intentJSON, err := json.Marshal(record.Intent)
	if err != nil {
		return orderIntentDoc{}, fmt.Errorf("marshal order intent: %w", err)
	}

	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = record.CreatedAt
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	return orderIntentDoc{
		ID:             record.ID,
		UserID:         record.UserID,
		IntentJSON:     string(intentJSON),
		IntentHash:     record.IntentHash,
		IntentType:     string(record.Intent.Type),
		Symbol:         intentSymbol(record.Intent),
		PlanID:         intentPlanID(record.Intent),
		CorrelationID:  record.CorrelationID,
		ConfirmationID: record.ConfirmationID,
		Status:         record.Status,
		ErrorMessage:   record.ErrorMessage,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      updatedAt,
	}, nil
}

func intentSymbol(intent domain.Intent) string {
	switch intent.Type {
	case domain.IntentOpen:
		if intent.Open != nil {
			return intent.Open.Symbol
		}
	case domain.IntentClose:
		if intent.Close != nil {
			return intent.Close.Symbol
		}
	case domain.IntentBreakeven:
		if intent.Breakeven != nil {
			return intent.Breakeven.Symbol
		}
	case domain.IntentTrail:
		if intent.Trail != nil {
			return intent.Trail.Symbol
		}
	case domain.IntentAdd:
		if intent.Add != nil {
			return intent.Add.Symbol
		}
	}
	return ""
}

func intentPlanID(intent domain.Intent) string {
	if intent.Type == domain.IntentOpen && intent.Open != nil {
		return intent.Open.PlanID
	}
	return ""
}

func (d orderIntentDoc) toIntentRecord() (orders.IntentRecord, error) {
	var intent domain.Intent
	if err := json.Unmarshal([]byte(d.IntentJSON), &intent); err != nil {
		return orders.IntentRecord{}, fmt.Errorf("unmarshal order intent: %w", err)
	}

	return orders.IntentRecord{
		ID:             d.ID,
		UserID:         d.UserID,
		Intent:         intent,
		IntentHash:     d.IntentHash,
		CorrelationID:  d.CorrelationID,
		ConfirmationID: d.ConfirmationID,
		Status:         d.Status,
		ErrorMessage:   d.ErrorMessage,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}, nil
}
