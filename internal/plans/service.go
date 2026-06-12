package plans

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Status struct {
	PlanID      string
	IntentCount int
	Symbols     []string
	LastUpdated time.Time
}

type StatusProvider interface {
	PlanStatus(ctx context.Context, userID int64, planID string) (Status, error)
}

type EmptyStatusProvider struct{}

func (EmptyStatusProvider) PlanStatus(ctx context.Context, userID int64, planID string) (Status, error) {
	return Status{PlanID: planID}, nil
}

type Service struct {
	provider StatusProvider
}

func NewService(provider StatusProvider) *Service {
	if provider == nil {
		provider = EmptyStatusProvider{}
	}
	return &Service{provider: provider}
}

func (s *Service) Text(ctx context.Context, userID int64, planID string) string {
	status, err := s.provider.PlanStatus(ctx, userID, planID)
	if err != nil {
		return "Plan " + planID + " status lookup failed: " + err.Error()
	}
	if status.IntentCount == 0 {
		return "Plan " + planID + " has no recorded intents yet."
	}

	symbols := "-"
	if len(status.Symbols) > 0 {
		symbols = strings.Join(status.Symbols, ", ")
	}
	updated := "-"
	if !status.LastUpdated.IsZero() {
		updated = status.LastUpdated.Format(time.RFC3339)
	}

	return fmt.Sprintf(
		"Plan %s\nRecorded intents: %d\nSymbols: %s\nLast updated: %s",
		status.PlanID,
		status.IntentCount,
		symbols,
		updated,
	)
}
