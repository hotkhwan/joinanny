package campaignexec

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"bottrade/internal/campaign"
	"bottrade/internal/signals"
)

// Notifier delivers campaign progress to the user (the bot sends it to their
// chat).
type Notifier interface {
	Notify(text string)
}

// Safety captures the runtime gates an autonomous campaign must pass. Every
// field must be true to run; the zero value runs nothing.
type Safety struct {
	Enabled            bool // CAMPAIGN_LIVE_ENABLED — explicit opt-in
	Testnet            bool // BINANCE_TESTNET
	RealTradingEnabled bool // REAL_TRADING_ENABLED — must be false
	DryRun             bool // DRY_RUN — must be false (no real fills to resolve)
}

// permit returns nil only when it is safe to run an autonomous campaign.
func (s Safety) permit() error {
	switch {
	case !s.Enabled:
		return errors.New("autonomous campaigns are disabled (set CAMPAIGN_LIVE_ENABLED=true)")
	case !s.Testnet:
		return errors.New("autonomous campaigns run on testnet only (BINANCE_TESTNET must be true)")
	case s.RealTradingEnabled:
		return errors.New("refusing to run: REAL_TRADING_ENABLED is true")
	case s.DryRun:
		return errors.New("autonomous campaigns need live testnet execution (DRY_RUN must be false)")
	}
	return nil
}

// ManagerDeps are the shared dependencies for building per-user campaign engines.
type ManagerDeps struct {
	Signals  campaign.SignalSource
	Advisor  signals.Advisor
	Placer   campaign.OrderPlacer
	Resolver campaign.CloseResolver
	Logger   *slog.Logger
}

// Manager starts and stops one autonomous campaign per user. It enforces the
// safety gates, runs the engine in the background, and reports progress through
// a Notifier. It is safe for concurrent use.
type Manager struct {
	deps   ManagerDeps
	safety Safety

	mu      sync.Mutex
	running map[int64]context.CancelFunc
}

// NewManager builds a campaign manager.
func NewManager(deps ManagerDeps, safety Safety) *Manager {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Manager{deps: deps, safety: safety, running: make(map[int64]context.CancelFunc)}
}

// IsRunning reports whether a campaign is active for the user.
func (m *Manager) IsRunning(userID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.running[userID]
	return ok
}

// Start launches an autonomous campaign for the user toward goal on symbol,
// reporting through notify. It returns an error if the safety gates fail, a
// campaign is already running for the user, or the engine cannot be built —
// nothing is placed in those cases.
func (m *Manager) Start(userID int64, symbol string, goal campaign.Goal, notify Notifier) error {
	if err := m.safety.permit(); err != nil {
		return err
	}
	if m.deps.Advisor == nil || m.deps.Signals == nil || m.deps.Placer == nil || m.deps.Resolver == nil {
		return errors.New("autonomous campaigns are not configured")
	}

	m.mu.Lock()
	if _, ok := m.running[userID]; ok {
		m.mu.Unlock()
		return errors.New("a campaign is already running; send /campaign stop first")
	}

	trader := campaign.NewLiveTrader(userID, m.deps.Placer, m.deps.Resolver, m.deps.Logger)
	engine, err := campaign.NewEngine(campaign.EngineConfig{
		Goal:                goal,
		Symbol:              symbol,
		Signals:             m.deps.Signals,
		Advisor:             m.deps.Advisor,
		Trader:              trader,
		Logger:              m.deps.Logger,
		MaxConsecutiveSkips: 20,
	})
	if err != nil {
		m.mu.Unlock()
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.running[userID] = cancel
	m.mu.Unlock()

	go m.run(ctx, userID, symbol, goal, engine, notify)
	return nil
}

// Stop cancels the user's running campaign, if any. In-flight trades finish
// resolving but no new trade is opened.
func (m *Manager) Stop(userID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cancel, ok := m.running[userID]
	if ok {
		cancel()
	}
	return ok
}

func (m *Manager) run(ctx context.Context, userID int64, symbol string, goal campaign.Goal, engine *campaign.Engine, notify Notifier) {
	defer func() {
		m.mu.Lock()
		delete(m.running, userID)
		m.mu.Unlock()
	}()

	notify.Notify(fmt.Sprintf("🤖 Autonomous campaign started on %s (testnet): target %s USDT, max %d trades. Send /campaign stop to halt.",
		symbol, goal.TargetProfitUSDT.String(), goal.MaxTrades))

	state, verdict, err := engine.Run(ctx)
	if err != nil {
		if ctx.Err() != nil {
			notify.Notify(fmt.Sprintf("⏹ Campaign stopped. Closed %d trades, PnL %s USDT.", state.TradesClosed, state.RealizedPnL.String()))
			return
		}
		m.deps.Logger.Warn("campaign run failed", "user_id", userID, "error", err)
		notify.Notify("⚠️ Campaign error: " + err.Error())
		return
	}

	notify.Notify(fmt.Sprintf("🏁 Campaign finished (%s): %d trades, PnL %s USDT.", verdict, state.TradesClosed, state.RealizedPnL.String()))
}
