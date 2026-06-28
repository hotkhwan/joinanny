package campaign

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"bottrade/internal/decimal"
	"bottrade/internal/signals"
)

type fakePlacer struct {
	placed []signals.Decision
	err    error
}

func (f *fakePlacer) Place(_ context.Context, _ int64, d signals.Decision) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.placed = append(f.placed, d)
	return "order-1", nil
}

type fakeResolver struct {
	pnl decimal.Decimal
	err error
}

func (f fakeResolver) AwaitClose(context.Context, string) (decimal.Decimal, error) {
	return f.pnl, f.err
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func openDecision() signals.Decision {
	return signals.Decision{Action: signals.ActionOpen, Symbol: "BTCUSDT", Side: "long"}
}

func TestLiveTraderPlacesAndResolves(t *testing.T) {
	placer := &fakePlacer{}
	trader := NewLiveTrader(7, placer, fakeResolver{pnl: decimal.MustParse("2.5")}, quietLogger())

	pnl, err := trader.Trade(context.Background(), openDecision())
	if err != nil {
		t.Fatalf("Trade: %v", err)
	}
	if pnl.String() != "2.5" {
		t.Fatalf("pnl = %s, want 2.5", pnl.String())
	}
	if len(placer.placed) != 1 || placer.placed[0].Symbol != "BTCUSDT" {
		t.Fatalf("placed = %+v, want one BTCUSDT order", placer.placed)
	}
}

func TestLiveTraderSkipsNonOpen(t *testing.T) {
	placer := &fakePlacer{}
	trader := NewLiveTrader(7, placer, fakeResolver{pnl: decimal.MustParse("99")}, quietLogger())

	pnl, err := trader.Trade(context.Background(), signals.Decision{Action: signals.ActionHold, Symbol: "BTCUSDT"})
	if err != nil {
		t.Fatalf("Trade: %v", err)
	}
	if !pnl.IsZero() || len(placer.placed) != 0 {
		t.Fatalf("a hold must place nothing and book zero; pnl=%s placed=%d", pnl.String(), len(placer.placed))
	}
}

func TestLiveTraderPropagatesPlaceError(t *testing.T) {
	trader := NewLiveTrader(7, &fakePlacer{err: errors.New("insufficient margin")}, fakeResolver{}, quietLogger())
	if _, err := trader.Trade(context.Background(), openDecision()); err == nil {
		t.Fatal("expected place error to propagate")
	}
}

func TestLiveTraderPropagatesResolveError(t *testing.T) {
	trader := NewLiveTrader(7, &fakePlacer{}, fakeResolver{err: errors.New("timeout")}, quietLogger())
	if _, err := trader.Trade(context.Background(), openDecision()); err == nil {
		t.Fatal("expected resolve error to propagate")
	}
}

func TestLiveTraderSatisfiesTrader(t *testing.T) {
	var _ Trader = NewLiveTrader(1, &fakePlacer{}, fakeResolver{}, quietLogger())
}
