package api

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
	"bottrade/internal/orders"
)

type fakePositionReader struct {
	// snapshots is a queue of position lists returned on successive polls.
	snapshots  [][]domain.Position
	call       int
	realized   orders.RealizedTrade
	realizedOK bool
	lastSide   string
	lastQty    decimal.Decimal
}

func (f *fakePositionReader) PositionsWithRequiredUserExecutor(_ context.Context, _ int64) ([]domain.Position, error) {
	i := f.call
	if i >= len(f.snapshots) {
		i = len(f.snapshots) - 1
	}
	f.call++
	return f.snapshots[i], nil
}

func (f *fakePositionReader) RealizedTradeWithRequiredUserExecutor(_ context.Context, _ int64, _, side string, _ time.Time, entryQty decimal.Decimal) (orders.RealizedTrade, bool, error) {
	f.lastSide = side
	f.lastQty = entryQty
	return f.realized, f.realizedOK, nil
}

func openPos(symbol, side string, amt int64) []domain.Position {
	return []domain.Position{{Symbol: symbol, Side: domain.PositionSide(side), Amount: decimal.NewFromInt(amt)}}
}

func newTestResolver(reader missionPositionReader) *userPositionResolver {
	return &userPositionResolver{
		orders: reader, userID: 7, poll: time.Millisecond, timeout: time.Hour,
		now:    func() time.Time { return time.Unix(1710000000, 0) },
		sleep:  func(context.Context, time.Duration) error { return nil },
		logger: slog.Default(),
	}
}

func TestUserPositionResolverReturnsRealizedPnLAfterFlat(t *testing.T) {
	reader := &fakePositionReader{
		snapshots: [][]domain.Position{
			openPos("BTCUSDT", "short", 1), // open
			openPos("BTCUSDT", "short", 1), // still open
			{},                             // flat → closed
		},
		realized:   orders.RealizedTrade{Symbol: "BTCUSDT", Side: "short", RealizedPnL: decimal.NewFromInt(3)},
		realizedOK: true,
	}
	pnl, err := newTestResolver(reader).AwaitClose(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("AwaitClose: %v", err)
	}
	if pnl.Cmp(decimal.NewFromInt(3)) != 0 {
		t.Fatalf("pnl = %s, want 3", pnl.String())
	}
	if reader.lastSide != "short" || reader.lastQty.Cmp(decimal.NewFromInt(1)) != 0 {
		t.Fatalf("realized query used side=%q qty=%s, want short/1 (learned from the open position)", reader.lastSide, reader.lastQty.String())
	}
}

func TestUserPositionResolverTimesOutIfNeverOpens(t *testing.T) {
	reader := &fakePositionReader{snapshots: [][]domain.Position{{}, {}}}
	r := newTestResolver(reader)
	r.timeout = 0 // deadline already passed on the first loop
	if _, err := r.AwaitClose(context.Background(), "BTCUSDT"); err == nil {
		t.Fatal("expected timeout when the position never opens")
	}
}
