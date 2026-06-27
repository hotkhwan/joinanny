package monitor

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
)

// StopState is the active stop-loss order for a symbol.
type StopState struct {
	Price        decimal.Decimal
	ClientAlgoID string
}

// Exchange is the slice of exchange behaviour the trailing runner needs. The
// Binance executor satisfies it; tests use a mock.
type Exchange interface {
	Positions(ctx context.Context) ([]domain.Position, error)
	CurrentStop(ctx context.Context, symbol string) (StopState, bool, error)
	MoveStopLoss(ctx context.Context, symbol string, side domain.PositionSide, newStop decimal.Decimal, oldClientAlgoID, newClientAlgoID string) error
}

// Runner periodically tightens the stop-loss of every open position according to
// the TrailPolicy. It is the live half of Phase 2 (the pure decision logic lives
// in ComputeStop).
type Runner struct {
	exchange Exchange
	policy   TrailPolicy
	interval time.Duration
	logger   *slog.Logger
	newID    func(symbol string, seq int) string
	seq      int
}

// NewRunner builds a trailing-stop runner.
func NewRunner(exchange Exchange, policy TrailPolicy, interval time.Duration, logger *slog.Logger) *Runner {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		exchange: exchange,
		policy:   policy,
		interval: interval,
		logger:   logger,
		newID:    defaultStopID,
	}
}

// Run trails stops every interval until the context is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	if !r.policy.Valid() {
		r.logger.Info("trailing stop monitor disabled (no policy)")
		<-ctx.Done()
		return ctx.Err()
	}
	r.logger.Info("trailing stop monitor started", "interval", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.Tick(ctx); err != nil {
				r.logger.Warn("trailing tick failed", "error", err)
			}
		}
	}
}

// Tick runs one trailing pass over all open positions. Exported for testing.
func (r *Runner) Tick(ctx context.Context) error {
	positions, err := r.exchange.Positions(ctx)
	if err != nil {
		return err
	}
	for _, position := range positions {
		if position.Amount.IsZero() {
			continue
		}
		stop, ok, err := r.exchange.CurrentStop(ctx, position.Symbol)
		if err != nil {
			r.logger.Warn("read current stop failed", "symbol", position.Symbol, "error", err)
			continue
		}
		if !ok {
			continue // nothing to trail (no protective stop on this position)
		}

		newStop, moved := ComputeStop(position.Side, position.EntryPrice, stop.Price, position.MarkPrice, r.policy)
		if !moved {
			continue
		}

		r.seq++
		newID := r.newID(position.Symbol, r.seq)
		if err := r.exchange.MoveStopLoss(ctx, position.Symbol, position.Side, newStop, stop.ClientAlgoID, newID); err != nil {
			r.logger.Warn("move stop loss failed", "symbol", position.Symbol, "error", err)
			continue
		}
		r.logger.Info("trailing stop moved",
			"symbol", position.Symbol,
			"side", position.Side,
			"from", stop.Price.String(),
			"to", newStop.String(),
		)
	}
	return nil
}

func defaultStopID(symbol string, seq int) string {
	return "tb_trail_" + symbol + "_" + strconv.Itoa(seq)
}
