package plans

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestServiceTextNoIntents(t *testing.T) {
	service := NewService(EmptyStatusProvider{})

	got := service.Text(context.Background(), 12345, "1")
	if got != "Plan 1 has no recorded intents yet." {
		t.Fatalf("text = %q, want no recorded intents", got)
	}
}

func TestServiceTextFormatsPlanStatus(t *testing.T) {
	service := NewService(fakeStatusProvider{
		status: Status{
			PlanID:      "2",
			IntentCount: 2,
			Symbols:     []string{"BTCUSDT", "ETHUSDT"},
			LastUpdated: time.Unix(1710000000, 0).UTC(),
		},
	})

	got := service.Text(context.Background(), 12345, "2")
	for _, want := range []string{"Plan 2", "Recorded intents: 2", "BTCUSDT, ETHUSDT"} {
		if !strings.Contains(got, want) {
			t.Fatalf("text = %q, want it to contain %q", got, want)
		}
	}
}

type fakeStatusProvider struct {
	status Status
}

func (f fakeStatusProvider) PlanStatus(ctx context.Context, userID int64, planID string) (Status, error) {
	return f.status, nil
}
