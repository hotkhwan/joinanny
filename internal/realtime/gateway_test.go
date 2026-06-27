package realtime

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
)

type fakeSource struct {
	frames [][]domain.Position
	call   int
}

func (f *fakeSource) Positions(context.Context) ([]domain.Position, error) {
	frame := f.frames[min(f.call, len(f.frames)-1)]
	f.call++
	return frame, nil
}

type recorder struct{ events []Event }

func (r *recorder) Publish(event Event) { r.events = append(r.events, event) }

func pos(symbol string, amount, mark, upnl string) domain.Position {
	return domain.Position{
		Symbol:           symbol,
		Side:             domain.PositionSideLong,
		Amount:           decimal.MustParse(amount),
		EntryPrice:       decimal.MustParse("100"),
		MarkPrice:        decimal.MustParse(mark),
		UnrealizedProfit: decimal.MustParse(upnl),
	}
}

func newTestGateway(source PositionSource, pub Publisher) *Gateway {
	return NewGateway(GatewayConfig{UserID: 7, Interval: time.Second}, source, pub,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestGatewayEmitsOpenChangeAndClose(t *testing.T) {
	source := &fakeSource{frames: [][]domain.Position{
		{pos("BTCUSDT", "0.1", "100", "0")},   // tick 1: new position
		{pos("BTCUSDT", "0.1", "105", "0.5")}, // tick 2: mark moved
		{},                                    // tick 3: closed
	}}
	rec := &recorder{}
	g := newTestGateway(source, rec)
	at := time.Unix(0, 0).UTC()

	for i := 0; i < 3; i++ {
		if err := g.Tick(context.Background(), at); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}

	if len(rec.events) != 3 {
		t.Fatalf("events = %d (%+v), want 3", len(rec.events), rec.events)
	}
	if rec.events[0].Type != EventPositionUpdate || rec.events[0].UserID != 7 {
		t.Fatalf("event 0 = %+v, want position_update for user 7", rec.events[0])
	}
	if rec.events[1].Type != EventPositionUpdate || rec.events[1].MarkPrice.String() != "105" {
		t.Fatalf("event 1 = %+v, want position_update at mark 105", rec.events[1])
	}
	if rec.events[2].Type != EventTradeClosed || rec.events[2].RealizedPnL.String() != "0.5" {
		t.Fatalf("event 2 = %+v, want trade_closed pnl 0.5 (last-seen)", rec.events[2])
	}
}

func TestGatewaySeedSuppressesReplayOfOpenPositions(t *testing.T) {
	source := &fakeSource{frames: [][]domain.Position{
		{pos("BTCUSDT", "0.1", "100", "0")}, // already open at startup
	}}
	rec := &recorder{}
	g := newTestGateway(source, rec)

	if err := g.seed(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// A tick over the same snapshot must not replay the already-open position.
	if err := g.Tick(context.Background(), time.Unix(0, 0).UTC()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(rec.events) != 0 {
		t.Fatalf("events = %+v, want none (seeded position not replayed)", rec.events)
	}
}

func TestGatewayNoDuplicateUpdateForUnchangedPosition(t *testing.T) {
	source := &fakeSource{frames: [][]domain.Position{
		{pos("BTCUSDT", "0.1", "100", "0")},
		{pos("BTCUSDT", "0.1", "100", "0")}, // identical
	}}
	rec := &recorder{}
	g := newTestGateway(source, rec)
	at := time.Unix(0, 0).UTC()

	_ = g.Tick(context.Background(), at)
	_ = g.Tick(context.Background(), at)

	if len(rec.events) != 1 {
		t.Fatalf("events = %d, want 1 (no duplicate for an unchanged position)", len(rec.events))
	}
}
