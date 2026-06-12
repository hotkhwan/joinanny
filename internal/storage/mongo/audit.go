package mongo

import (
	"context"
	"fmt"
	"time"

	"bottrade/internal/audit"
)

type auditEventDoc struct {
	Type          string            `bson:"type"`
	Source        string            `bson:"source"`
	UserID        int64             `bson:"user_id"`
	CorrelationID string            `bson:"correlation_id,omitempty"`
	Metadata      map[string]string `bson:"metadata,omitempty"`
	CreatedAt     time.Time         `bson:"created_at"`
}

func (s *Store) Record(ctx context.Context, event audit.Event) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	doc := auditEventDoc{
		Type:          event.Type,
		Source:        event.Source,
		UserID:        event.UserID,
		CorrelationID: event.CorrelationID,
		Metadata:      event.Metadata,
		CreatedAt:     event.CreatedAt,
	}
	if _, err := s.auditEvents.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}
