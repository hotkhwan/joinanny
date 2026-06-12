package orders

import (
	"context"
	"strings"
	"testing"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
)

func TestStatusServiceNoPositions(t *testing.T) {
	service := NewStatusService(EmptyPositionProvider{})

	if got := service.Text(context.Background()); got != "No open positions." {
		t.Fatalf("status = %q, want no positions", got)
	}
}

func TestStatusServiceFormatsPositions(t *testing.T) {
	service := NewStatusService(fakePositionProvider{
		positions: []domain.Position{
			{
				Symbol:           "BTCUSDT",
				Side:             domain.PositionSideLong,
				Amount:           decimal.MustParse("0.01"),
				EntryPrice:       decimal.MustParse("67500"),
				MarkPrice:        decimal.MustParse("68000"),
				UnrealizedProfit: decimal.MustParse("5"),
				Leverage:         3,
				MarginType:       "isolated",
			},
		},
	})

	got := service.Text(context.Background())
	for _, want := range []string{"Open positions", "LONG BTCUSDT", "Unrealized PnL: 5 USDT"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status = %q, want it to contain %q", got, want)
		}
	}
}

type fakePositionProvider struct {
	positions []domain.Position
}

func (f fakePositionProvider) Positions(ctx context.Context) ([]domain.Position, error) {
	return f.positions, nil
}
