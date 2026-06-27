package realtime

import (
	"context"
	"log/slog"
	"time"

	"bottrade/internal/domain"
)

// PositionSource reads the current open positions. *binance.Executor satisfies
// it via Positions.
type PositionSource interface {
	Positions(ctx context.Context) ([]domain.Position, error)
}

// Publisher receives gateway events. *Broadcaster satisfies it; tests use a
// recorder.
type Publisher interface {
	Publish(event Event)
}

// GatewayConfig configures the polling gateway.
type GatewayConfig struct {
	// UserID labels the events; the owner's Telegram id for a single-account
	// gateway.
	UserID int64
	// Interval is the poll period. Defaults to 3s.
	Interval time.Duration
}

// Gateway snapshots open positions on an interval and publishes an event when a
// position opens, changes materially, or closes. It is the live, dependency-free
// source of realtime updates: every poll diffs against the previous snapshot, so
// a stop-loss or take-profit fill (which never reaches the order service) still
// produces a trade_closed event.
//
// PnL on close is the last-observed unrealized PnL of the vanished position — an
// estimate bounded by the poll interval. For exact realized PnL, the campaign
// Trader queries Binance income history; a future user-data WebSocket carries the
// settled figure directly (see ParseUserDataEvent).
type Gateway struct {
	source    PositionSource
	publisher Publisher
	userID    int64
	interval  time.Duration
	logger    *slog.Logger

	last map[string]domain.Position
}

// NewGateway builds a polling gateway. source and publisher are required.
func NewGateway(cfg GatewayConfig, source PositionSource, publisher Publisher, logger *slog.Logger) *Gateway {
	if cfg.Interval <= 0 {
		cfg.Interval = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Gateway{
		source:    source,
		publisher: publisher,
		userID:    cfg.UserID,
		interval:  cfg.Interval,
		logger:    logger,
		last:      make(map[string]domain.Position),
	}
}

// Run polls until the context is cancelled. The first tick seeds the baseline
// snapshot silently (no events for already-open positions) so a restart does not
// replay history.
func (g *Gateway) Run(ctx context.Context) error {
	g.logger.Info("realtime gateway started", "interval", g.interval, "user_id", g.userID)
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	// Seed the baseline without emitting, so positions already open at startup
	// are not replayed as fresh updates.
	if err := g.seed(ctx); err != nil {
		g.logger.Warn("realtime gateway seed failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := g.Tick(ctx, time.Now().UTC()); err != nil {
				g.logger.Warn("realtime tick failed", "error", err)
			}
		}
	}
}

func (g *Gateway) seed(ctx context.Context) error {
	positions, err := g.source.Positions(ctx)
	if err != nil {
		return err
	}
	g.last = indexBySymbol(positions)
	return nil
}

// Tick runs one poll: diff the current positions against the last snapshot,
// publish updates for new/changed positions and closes for vanished ones, then
// adopt the new snapshot. Exported for tests; at is the event timestamp.
func (g *Gateway) Tick(ctx context.Context, at time.Time) error {
	positions, err := g.source.Positions(ctx)
	if err != nil {
		return err
	}
	current := indexBySymbol(positions)

	for symbol, position := range current {
		previous, existed := g.last[symbol]
		if !existed || changed(previous, position) {
			g.publisher.Publish(Event{
				Type:          EventPositionUpdate,
				UserID:        g.userID,
				Symbol:        symbol,
				Side:          string(position.Side),
				Amount:        position.Amount,
				EntryPrice:    position.EntryPrice,
				MarkPrice:     position.MarkPrice,
				UnrealizedPnL: position.UnrealizedProfit,
				At:            at,
			})
		}
	}

	for symbol, previous := range g.last {
		if _, stillOpen := current[symbol]; stillOpen {
			continue
		}
		g.publisher.Publish(Event{
			Type:        EventTradeClosed,
			UserID:      g.userID,
			Symbol:      symbol,
			Side:        string(previous.Side),
			EntryPrice:  previous.EntryPrice,
			MarkPrice:   previous.MarkPrice,
			RealizedPnL: previous.UnrealizedProfit, // last-seen estimate; see type doc
			At:          at,
		})
	}

	g.last = current
	return nil
}

// changed reports a material difference worth publishing: a size change or a
// mark-price move. Mark price moves every tick on a live position, so this keeps
// the stream lively without flooding on identical snapshots.
func changed(previous, current domain.Position) bool {
	return !previous.Amount.Equal(current.Amount) || !previous.MarkPrice.Equal(current.MarkPrice)
}

func indexBySymbol(positions []domain.Position) map[string]domain.Position {
	index := make(map[string]domain.Position, len(positions))
	for _, position := range positions {
		if position.Amount.IsZero() {
			continue
		}
		index[position.Symbol] = position
	}
	return index
}
