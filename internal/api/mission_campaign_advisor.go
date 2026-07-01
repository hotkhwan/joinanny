package api

import (
	"context"
	"encoding/json"
	"fmt"

	"bottrade/internal/decimal"
	"bottrade/internal/signals"
	"bottrade/internal/strategy/annybasic"
)

// annyBasicObserveFunc yields the current ANNY Basic observation and reference
// entry price for a symbol. It is injected so the advisor stays pure and
// unit-testable: production wraps the live 15m/1m candle fetch (see
// annyBasicLiveDecision), while tests stub a deterministic observation.
type annyBasicObserveFunc func(ctx context.Context, symbol string) (annybasic.Observation, float64, error)

// missionCampaignAdvisor is a stateful signals.Advisor that drives a multi-trade
// mission with ANNY Basic. Unlike the single-shot armed path — which always
// evaluates against a zero State (mission.go annyBasicLiveDecision) — it
// accumulates realized outcomes across the campaign, so the model's own stop
// rules (profit target, two consecutive losses, and the trade cap in
// annybasic.Evaluate) actually fire live instead of being dead code.
//
// It is NOT safe for concurrent Decide/Record. The campaign runner calls them
// serially — one trade resolves (Record) before the next Decide — matching how
// campaign.Engine drives its Advisor and Trader.
type missionCampaignAdvisor struct {
	observe        annyBasicObserveFunc
	capitalUSDT    json.Number
	leverageUsePct int
	maxLeverage    int

	state      annybasic.State
	stopped    bool
	stopReason string
}

// newMissionCampaignAdvisor builds an advisor seeded with initial (possibly
// rehydrated) campaign state, so a mission resumed after a restart keeps counting
// from its persisted TradesClosed / ConsecutiveLosses / RealizedPnL.
func newMissionCampaignAdvisor(observe annyBasicObserveFunc, capitalUSDT json.Number, leverageUsePct, maxLeverage int, initial annybasic.State) *missionCampaignAdvisor {
	if maxLeverage <= 0 {
		maxLeverage = missionMaxLeverage
	}
	return &missionCampaignAdvisor{
		observe:        observe,
		capitalUSDT:    capitalUSDT,
		leverageUsePct: leverageUsePct,
		maxLeverage:    maxLeverage,
		state:          initial,
	}
}

// Decide implements signals.Advisor. It returns ActionOpen with a fully capped
// intent (size clamped by missionSize, leverage by missionLeverageFor/clampLeverage,
// protective bracket by missionBracket) when ANNY Basic proposes a side, and
// ActionHold otherwise. When the model's campaign-stop policy triggers, Decide
// records the stop (surfaced via Stopped) and holds so the runner ends the mission
// rather than opening another trade.
func (a *missionCampaignAdvisor) Decide(ctx context.Context, signal signals.MarketSignal) (signals.Decision, error) {
	obs, price, err := a.observe(ctx, signal.Symbol)
	if err != nil {
		return signals.Decision{}, err
	}
	decision := annybasic.Evaluate(obs, a.state, a.maxLeverage)
	if decision.Stop {
		a.stopped = true
		a.stopReason = decision.Reason
		return signals.Decision{Action: signals.ActionHold, Symbol: signal.Symbol, Reason: decision.Reason}, nil
	}
	if decision.Side == annybasic.SideNone {
		return signals.Decision{Action: signals.ActionHold, Symbol: signal.Symbol, Reason: decision.Reason}, nil
	}
	side := string(decision.Side)
	sl, tp := missionBracket(side, price)
	size := missionSize(a.capitalUSDT)
	leverage := missionLeverageFor(a.leverageUsePct, minPositive(a.maxLeverage, decision.MaxLeverage))
	return signals.Decision{
		Action:     signals.ActionOpen,
		Symbol:     signal.Symbol,
		Strategy:   missionStrategyID(annybasic.ID),
		Side:       side,
		Leverage:   leverage,
		Entry:      trimPrice(price),
		StopLoss:   trimPrice(sl),
		TakeProfit: trimPrice(tp),
		SizeUSDT:   fmt.Sprintf("%.2f", size),
		Reason:     annybasic.ID + " v" + annybasic.Version + " · " + decision.Reason,
	}, nil
}

// Record folds one closed trade's realized PnL back into the campaign state so the
// next Decide sees the true TradesClosed / ConsecutiveLosses / RealizedPnL. A
// negative PnL is a loss; breakeven or profit resets the loss streak. This is the
// seam that makes annybasic.Evaluate's risk stops live — a bug here silently
// disables the two-loss and trade-cap protections, so it is tested directly.
func (a *missionCampaignAdvisor) Record(pnl decimal.Decimal) {
	a.state.TradesClosed++
	a.state.RealizedPnLUSDT = a.state.RealizedPnLUSDT.Add(pnl)
	if pnl.Cmp(decimal.Zero()) < 0 {
		a.state.ConsecutiveLosses++
	} else {
		a.state.ConsecutiveLosses = 0
	}
}

// State returns the accumulated campaign state, for durable persistence and
// rehydrate on restart.
func (a *missionCampaignAdvisor) State() annybasic.State { return a.state }

// Stopped reports whether the model's campaign-stop policy has fired this mission,
// with the model's reason (e.g. "profit target reached").
func (a *missionCampaignAdvisor) Stopped() (bool, string) { return a.stopped, a.stopReason }
