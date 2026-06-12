package audit

import (
	"context"
	"time"
)

type Event struct {
	Type          string
	Source        string
	UserID        int64
	CorrelationID string
	Metadata      map[string]string
	CreatedAt     time.Time
}

type Recorder interface {
	Record(ctx context.Context, event Event) error
}

type NoopRecorder struct{}

func (NoopRecorder) Record(ctx context.Context, event Event) error {
	return nil
}
