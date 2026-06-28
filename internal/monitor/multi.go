package monitor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"time"
)

// ExchangeProvider resolves the trailing Exchange for one user's own account, so
// the trailer can manage every user's positions on their own key.
type ExchangeProvider interface {
	ExchangeFor(ctx context.Context, userKey string) (Exchange, bool, error)
}

// UserLister lists the user keys that may have open positions to trail (e.g. the
// distinct owners of stored Binance credentials).
type UserLister interface {
	TradingUserKeys(ctx context.Context) ([]string, error)
}

// MultiTrailer trails stops for every user's own positions. It is the multi-
// tenant sibling of Runner: where Runner manages a single platform account, this
// fans out per user so live Missions placed on per-user keys get autonomous
// break-even + trailing management. The decision logic is the same ComputeStop.
type MultiTrailer struct {
	provider ExchangeProvider
	users    UserLister
	policy   TrailPolicy
	interval time.Duration
	logger   *slog.Logger
	nonce    string // per-process, so trail order IDs don't collide across restarts
	seq      int
}

// trailNonce returns a short random hex string, or a time-based fallback.
func trailNonce() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano()&0xffffffff, 16)
	}
	return hex.EncodeToString(b)
}

// NewMultiTrailer builds a per-user trailing-stop trailer.
func NewMultiTrailer(provider ExchangeProvider, users UserLister, policy TrailPolicy, interval time.Duration, logger *slog.Logger) *MultiTrailer {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MultiTrailer{provider: provider, users: users, policy: policy, interval: interval, logger: logger, nonce: trailNonce()}
}

// Run trails every user's stops each interval until the context is cancelled.
func (m *MultiTrailer) Run(ctx context.Context) error {
	if !m.policy.Valid() || m.provider == nil || m.users == nil {
		m.logger.Info("per-user trailing monitor disabled (no policy/provider)")
		<-ctx.Done()
		return ctx.Err()
	}
	m.logger.Info("per-user trailing monitor started", "interval", m.interval)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := m.Tick(ctx); err != nil {
				m.logger.Warn("per-user trailing tick failed", "error", err)
			}
		}
	}
}

// Tick runs one trailing pass over every user's positions. Exported for testing.
func (m *MultiTrailer) Tick(ctx context.Context) error {
	keys, err := m.users.TradingUserKeys(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		ex, ok, err := m.provider.ExchangeFor(ctx, key)
		if err != nil {
			m.logger.Warn("trailing: resolve user exchange failed", "user", key, "error", err)
			continue
		}
		if !ok || ex == nil {
			continue
		}
		m.trailUser(ctx, key, ex)
	}
	return nil
}

// trailUser runs one trailing pass over a single user's positions. A failure on
// one symbol or user never aborts the others.
func (m *MultiTrailer) trailUser(ctx context.Context, key string, ex Exchange) {
	positions, err := ex.Positions(ctx)
	if err != nil {
		m.logger.Warn("trailing: read positions failed", "user", key, "error", err)
		return
	}
	for _, position := range positions {
		if position.Amount.IsZero() {
			continue
		}
		stop, ok, err := ex.CurrentStop(ctx, position.Symbol)
		if err != nil {
			m.logger.Warn("trailing: read stop failed", "user", key, "symbol", position.Symbol, "error", err)
			continue
		}
		if !ok {
			continue // no protective stop to move
		}
		newStop, moved := ComputeStop(position.Side, position.EntryPrice, stop.Price, position.MarkPrice, m.policy)
		if !moved {
			continue
		}
		m.seq++
		newID := "tb_trail_" + m.nonce + "_" + position.Symbol + "_" + strconv.Itoa(m.seq)
		if err := ex.MoveStopLoss(ctx, position.Symbol, position.Side, newStop, stop.ClientAlgoID, newID); err != nil {
			m.logger.Warn("trailing: move stop failed", "user", key, "symbol", position.Symbol, "error", err)
			continue
		}
		// Log the move, never the policy values — the edge stays secret.
		m.logger.Info("trailing stop moved", "symbol", position.Symbol, "side", position.Side, "to", newStop.String())
	}
}
