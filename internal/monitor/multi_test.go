package monitor

import (
	"context"
	"testing"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
)

type mockProvider struct {
	exchanges map[string]Exchange
}

func (p mockProvider) ExchangeFor(_ context.Context, key string) (Exchange, bool, error) {
	ex, ok := p.exchanges[key]
	return ex, ok, nil
}

type mockLister struct{ keys []string }

func (l mockLister) TradingUserKeys(context.Context) ([]string, error) { return l.keys, nil }

func trailerPolicy() TrailPolicy {
	return TrailPolicy{ActivatePct: decimal.MustParse("1"), TrailGapPct: decimal.MustParse("1")}
}

func TestMultiTrailerTrailsEachUsersOwnPositions(t *testing.T) {
	// User A: a profitable long with a protective stop → should be tightened.
	exA := &mockExchange{
		positions: []domain.Position{longPosition("BTCUSDT", "60000", "61000")}, // +1.67%
		stops:     map[string]StopState{"BTCUSDT": {Price: decimal.MustParse("58000"), ClientAlgoID: "a-sl"}},
	}
	// User B: a long that is barely up (below ActivatePct) → no move.
	exB := &mockExchange{
		positions: []domain.Position{longPosition("ETHUSDT", "3000", "3005")}, // +0.17%
		stops:     map[string]StopState{"ETHUSDT": {Price: decimal.MustParse("2900"), ClientAlgoID: "b-sl"}},
	}
	mt := NewMultiTrailer(
		mockProvider{exchanges: map[string]Exchange{"tg:1": exA, "tg:2": exB}},
		mockLister{keys: []string{"tg:1", "tg:2"}},
		trailerPolicy(), 0, nil,
	)
	if err := mt.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(exA.moves) != 1 {
		t.Fatalf("user A moves = %d, want 1 (BE/trail)", len(exA.moves))
	}
	if len(exB.moves) != 0 {
		t.Fatalf("user B moves = %d, want 0 (below activate)", len(exB.moves))
	}
}

func TestMultiTrailerDisabledWithoutPolicy(t *testing.T) {
	ex := &mockExchange{
		positions: []domain.Position{longPosition("BTCUSDT", "60000", "61000")},
		stops:     map[string]StopState{"BTCUSDT": {Price: decimal.MustParse("58000")}},
	}
	// Invalid (zero) policy → ComputeStop never moves anything.
	mt := NewMultiTrailer(mockProvider{exchanges: map[string]Exchange{"tg:1": ex}}, mockLister{keys: []string{"tg:1"}}, TrailPolicy{}, 0, nil)
	if err := mt.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(ex.moves) != 0 {
		t.Fatalf("moves = %d, want 0 with no policy", len(ex.moves))
	}
}
