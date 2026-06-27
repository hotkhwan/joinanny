package campaign

import (
	"context"
	"fmt"
	"log/slog"

	"bottrade/internal/decimal"
	"bottrade/internal/signals"
)

// OrderPlacer places the entry + protective orders for an open decision on a
// user's account and returns once the order is accepted. The order service
// implements it by preparing and confirming the intent against the user's
// per-user executor.
type OrderPlacer interface {
	Place(ctx context.Context, userID int64, decision signals.Decision) (ref string, err error)
}

// CloseResolver blocks until the position on symbol goes flat (stop-loss,
// take-profit, or manual close) and returns the realized PnL. A subscriber on
// the realtime position gateway implements it.
type CloseResolver interface {
	AwaitClose(ctx context.Context, symbol string) (decimal.Decimal, error)
}

// LiveTrader implements Trader by placing a real order and waiting for it to
// resolve — the autonomous-execution half of Phase 4. It closes the campaign
// loop: the engine asks the advisor, LiveTrader executes and blocks until the
// trade resolves, and the engine tallies the realized PnL.
//
// SAFETY: this places real exchange orders. It must only ever be constructed
// with a placer bound to a testnet, real-trading-disabled executor; the caller
// is responsible for those gates (see the campaign command's preconditions).
type LiveTrader struct {
	userID   int64
	placer   OrderPlacer
	resolver CloseResolver
	logger   *slog.Logger
}

// NewLiveTrader builds a live trader for one user.
func NewLiveTrader(userID int64, placer OrderPlacer, resolver CloseResolver, logger *slog.Logger) *LiveTrader {
	if logger == nil {
		logger = slog.Default()
	}
	return &LiveTrader{userID: userID, placer: placer, resolver: resolver, logger: logger}
}

// Trade places the decision's order and blocks until it resolves, returning the
// realized PnL. A non-open decision is a no-op worth zero PnL (the engine treats
// it as "stayed in cash"), so the campaign's skip guard still applies upstream.
func (t *LiveTrader) Trade(ctx context.Context, decision signals.Decision) (decimal.Decimal, error) {
	if decision.Action != signals.ActionOpen {
		return decimal.Zero(), nil
	}

	ref, err := t.placer.Place(ctx, t.userID, decision)
	if err != nil {
		return decimal.Zero(), fmt.Errorf("campaign: place order: %w", err)
	}
	t.logger.Info("campaign live trade placed", "user_id", t.userID, "symbol", decision.Symbol, "ref", ref)

	pnl, err := t.resolver.AwaitClose(ctx, decision.Symbol)
	if err != nil {
		return decimal.Zero(), fmt.Errorf("campaign: await close for %s: %w", decision.Symbol, err)
	}
	t.logger.Info("campaign live trade resolved", "user_id", t.userID, "symbol", decision.Symbol, "pnl", pnl.String())
	return pnl, nil
}
