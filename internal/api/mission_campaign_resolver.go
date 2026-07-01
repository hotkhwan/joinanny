package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
	"bottrade/internal/orders"
)

// missionPositionReader is the slice of *orders.Service the campaign resolver
// needs — both scoped to the user's OWN executor, so a mission never resolves
// against another account's position stream.
type missionPositionReader interface {
	PositionsWithRequiredUserExecutor(ctx context.Context, userID int64) ([]domain.Position, error)
	RealizedTradeWithRequiredUserExecutor(ctx context.Context, userID int64, symbol, side string, since time.Time, entryQty decimal.Decimal) (orders.RealizedTrade, bool, error)
}

// userPositionResolver blocks until the mission's own symbol position goes flat
// and returns its realized PnL, polling the USER's executor. It replaces the
// shared realtime broadcaster resolver, which was scoped to a single (admin)
// account and matched on symbol only — so it could miss (or mis-attribute) a
// user's close and stall the multi-trade loop. It implements campaign.CloseResolver.
type userPositionResolver struct {
	orders  missionPositionReader
	userID  int64
	poll    time.Duration
	timeout time.Duration
	now     func() time.Time
	sleep   func(ctx context.Context, d time.Duration) error
	logger  *slog.Logger
}

func newUserPositionResolver(reader missionPositionReader, userID int64, logger *slog.Logger) *userPositionResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &userPositionResolver{
		orders:  reader,
		userID:  userID,
		poll:    campaignResolvePollInterval,
		timeout: campaignTradeResolveTimeout,
		now:     time.Now,
		sleep:   sleepCtx,
		logger:  logger,
	}
}

// AwaitClose watches the user's position for symbol: once it has been seen open
// and then goes flat, it reads the realized round-trip PnL from the user's
// executor. A trade that never appears (unfilled) times out.
func (r *userPositionResolver) AwaitClose(ctx context.Context, symbol string) (decimal.Decimal, error) {
	since := r.now()
	deadline := since.Add(r.timeout)
	var side string
	var entryQty decimal.Decimal
	opened := false

	for {
		positions, err := r.orders.PositionsWithRequiredUserExecutor(ctx, r.userID)
		if err != nil {
			r.logger.Warn("mission campaign resolver positions read failed", "user_id", r.userID, "symbol", symbol, "error", err)
		} else if pos, has := openPositionFor(positions, symbol); has {
			opened = true
			side = string(pos.Side)
			entryQty = pos.Amount.Abs()
		} else if opened {
			// Was open, now flat → the trade closed. Read its realized PnL.
			rt, ok, rerr := r.orders.RealizedTradeWithRequiredUserExecutor(ctx, r.userID, symbol, side, since, entryQty)
			if rerr != nil {
				r.logger.Warn("mission campaign resolver realized read failed", "user_id", r.userID, "symbol", symbol, "error", rerr)
				return decimal.Zero(), rerr
			}
			if ok {
				return rt.RealizedPnL, nil
			}
			// Closed but no realized trade found (e.g. dust) → treat as breakeven.
			return decimal.Zero(), nil
		}

		if !r.now().Before(deadline) {
			return decimal.Zero(), fmt.Errorf("mission campaign: timed out waiting for %s to close", symbol)
		}
		if err := r.sleep(ctx, r.poll); err != nil {
			return decimal.Zero(), err
		}
	}
}

// openPositionFor returns the non-flat position for symbol, if any.
func openPositionFor(positions []domain.Position, symbol string) (domain.Position, bool) {
	for _, p := range positions {
		if p.Symbol == symbol && !p.Amount.IsZero() {
			return p, true
		}
	}
	return domain.Position{}, false
}
