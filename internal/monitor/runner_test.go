package monitor

import (
	"context"
	"errors"
	"testing"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
)

type moveCall struct {
	symbol  string
	side    domain.PositionSide
	newStop string
	oldID   string
	newID   string
}

type mockExchange struct {
	positions []domain.Position
	stops     map[string]StopState // symbol -> current stop; absent => no stop
	moves     []moveCall
	moveErr   error
}

func (m *mockExchange) Positions(context.Context) ([]domain.Position, error) {
	return m.positions, nil
}

func (m *mockExchange) CurrentStop(_ context.Context, symbol string) (StopState, bool, error) {
	s, ok := m.stops[symbol]
	return s, ok, nil
}

func (m *mockExchange) MoveStopLoss(_ context.Context, symbol string, side domain.PositionSide, newStop decimal.Decimal, oldID, newID string) error {
	if m.moveErr != nil {
		return m.moveErr
	}
	m.moves = append(m.moves, moveCall{symbol, side, newStop.String(), oldID, newID})
	return nil
}

func longPosition(symbol, entry, mark string) domain.Position {
	return domain.Position{
		Symbol:     symbol,
		Side:       domain.PositionSideLong,
		Amount:     decimal.MustParse("0.01"),
		EntryPrice: decimal.MustParse(entry),
		MarkPrice:  decimal.MustParse(mark),
	}
}

func runner(ex Exchange) *Runner {
	r := NewRunner(ex, TrailPolicy{ActivatePct: decimal.MustParse("1"), TrailGapPct: decimal.MustParse("1")}, 0, nil)
	r.newID = func(symbol string, seq int) string { return "new-" + symbol }
	return r
}

func TestTickMovesStopWhenProfitable(t *testing.T) {
	ex := &mockExchange{
		positions: []domain.Position{longPosition("BTCUSDT", "60000", "61000")}, // +1.67%
		stops:     map[string]StopState{"BTCUSDT": {Price: decimal.MustParse("58000"), ClientAlgoID: "old-sl"}},
	}
	if err := runner(ex).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(ex.moves) != 1 {
		t.Fatalf("moves = %d, want 1", len(ex.moves))
	}
	mv := ex.moves[0]
	if mv.symbol != "BTCUSDT" || mv.newStop != "60400" || mv.oldID != "old-sl" || mv.newID != "new-BTCUSDT" {
		t.Fatalf("move = %+v, want BTCUSDT 60400 old-sl new-BTCUSDT", mv)
	}
}

func TestTickSkipsWhenNotProfitableOrNoStop(t *testing.T) {
	ex := &mockExchange{
		positions: []domain.Position{
			longPosition("BTCUSDT", "60000", "60100"), // +0.16% < activate
			longPosition("ETHUSDT", "3000", "3300"),   // profitable but no stop tracked
		},
		stops: map[string]StopState{"BTCUSDT": {Price: decimal.MustParse("59000"), ClientAlgoID: "btc-sl"}},
	}
	if err := runner(ex).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(ex.moves) != 0 {
		t.Fatalf("moves = %d, want 0 (not profitable / no stop)", len(ex.moves))
	}
}

func TestTickSkipsClosedPosition(t *testing.T) {
	closed := longPosition("BTCUSDT", "60000", "61000")
	closed.Amount = decimal.Zero()
	ex := &mockExchange{
		positions: []domain.Position{closed},
		stops:     map[string]StopState{"BTCUSDT": {Price: decimal.MustParse("58000"), ClientAlgoID: "old"}},
	}
	if err := runner(ex).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(ex.moves) != 0 {
		t.Fatalf("moves = %d, want 0 (position closed)", len(ex.moves))
	}
}

func TestTickContinuesAfterMoveError(t *testing.T) {
	ex := &mockExchange{
		positions: []domain.Position{longPosition("BTCUSDT", "60000", "61000")},
		stops:     map[string]StopState{"BTCUSDT": {Price: decimal.MustParse("58000"), ClientAlgoID: "old"}},
		moveErr:   errors.New("exchange down"),
	}
	// A failing move must not crash the tick.
	if err := runner(ex).Tick(context.Background()); err != nil {
		t.Fatalf("Tick returned error, want nil: %v", err)
	}
}
