package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"bottrade/internal/campaign"
	"bottrade/internal/campaignexec"
	"bottrade/internal/decimal"
	"bottrade/internal/strategy/annybasic"
)

const (
	// campaignEntryCutoffFraction stops opening new trades in the final tenth of
	// the plan window so a late entry cannot outlive the mission.
	campaignEntryCutoffFraction = 10
	// campaignPerTradeMaxHold bounds how long a single re-entry may stay open before
	// its durable timed close flushes it, so trades cycle instead of one held to the
	// window end. Capped to the remaining window at schedule time.
	campaignPerTradeMaxHold = 30 * time.Minute
	// campaignTradeResolveTimeout bounds RealtimeResolver's wait for one trade to
	// close; SL/TP or the per-trade timed close resolve well within it.
	campaignTradeResolveTimeout      = 40 * time.Minute
	maxActiveCampaignMissionsPerUser = 3
)

func newCampaignMissionID() (string, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create campaign mission id: %w", err)
	}
	return "cmp_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// campaignMissionRuntimeAllowed reuses the armed-mission runtime gate: campaigns
// place real testnet orders and must never run outside the testnet, real-trading-off
// envelope.
func (s *Server) campaignMissionRuntimeAllowed() bool {
	return s.armedMissionRuntimeAllowed() && s.stream != nil
}

// campaignMissionGate is re-asserted before EVERY re-entry (staged and confirmed),
// so a mid-mission tier downgrade, key removal, or gate flip halts new orders.
func (s *Server) campaignMissionGate(userKey string) func(ctx context.Context) bool {
	return func(ctx context.Context) bool {
		if !s.campaignMissionRuntimeAllowed() || s.orders == nil {
			return false
		}
		if !s.hasActiveKeyForSubject(ctx, userKey) {
			return false
		}
		allowed, _ := s.allow(ctx, userKey, "mission")
		return allowed
	}
}

// annyBasicObserve fetches the closed 15m/1m candles and builds the current ANNY
// Basic observation plus a reference entry price, for the campaign advisor. On
// insufficient indicator data it returns a neutral observation (advisor holds)
// rather than an error, so a warming-up market does not spam the run log.
func (s *Server) annyBasicObserve(ctx context.Context, symbol string) (annybasic.Observation, float64, error) {
	exec, err := s.market.Candles(ctx, symbol, armedMissionExecutionInterval, armedMissionExecutionBars)
	if err != nil || len(exec) == 0 {
		if err == nil {
			err = fmt.Errorf("no execution candles for %s", symbol)
		}
		return annybasic.Observation{}, 0, err
	}
	price := exec[len(exec)-1].Close
	main, err := s.market.Candles(ctx, symbol, "15m", 200)
	if err != nil {
		return annybasic.Observation{}, 0, err
	}
	obs, err := annybasic.ObserveAt(main, exec, len(exec)-1)
	if err != nil {
		// Insufficient indicator data → hold, not fail.
		return annybasic.Observation{}, price, nil
	}
	return obs, price, nil
}

func campaignGoalFor(mission CampaignMission) campaign.Goal {
	capital, _ := decimal.Parse(defaultString(mission.CapitalUSDT, "100"))
	target, _ := decimal.Parse(defaultString(mission.TargetProfitUSDT, "5"))
	maxTrades := mission.MaxTrades
	if maxTrades <= 0 {
		maxTrades = 15
	}
	drawdown := decimal.Zero()
	if mission.CapitalRiskPct > 0 {
		if d, err := capital.Mul(decimal.NewFromInt(int64(mission.CapitalRiskPct))).QuoFloor(decimal.NewFromInt(100), 8); err == nil {
			drawdown = d
		}
	}
	return campaign.Goal{
		CapitalUSDT:        capital,
		TargetProfitUSDT:   target,
		RewardPerTradeUSDT: decimal.NewFromInt(2),
		RiskPerTradeUSDT:   decimal.NewFromInt(1),
		AssumedWinRate:     55,
		MaxTrades:          maxTrades,
		MaxDrawdownUSDT:    drawdown,
	}
}

func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// startCampaignMissionRunners rehydrates running missions on boot and sweeps
// orphans whose window elapsed while the process was down (mirrors
// startArmedMissionWatchers).
func (s *Server) startCampaignMissionRunners(ctx context.Context) int {
	if s.campaignMissions == nil {
		return 0
	}
	now := time.Now().UTC()
	if swept, err := s.campaignMissions.ExpireStale(ctx, now); err != nil {
		s.logger.Warn("campaign mission expire-stale failed", "error", err)
	} else if swept > 0 {
		s.logger.Info("campaign missions expired on boot (stale orphans)", "count", swept)
	}
	rows, err := s.campaignMissions.ListActive(ctx, now)
	if err != nil {
		s.logger.Warn("campaign mission rehydrate failed", "error", err)
		return 0
	}
	for _, row := range rows {
		s.startCampaignMissionRunner(ctx, row)
	}
	return len(rows)
}

// startCampaignMissionRunner assembles the windowed runner (advisor + required-user
// idempotency placer + realtime resolver + paced signals) and runs it in the
// background until a stop rule fires or the plan window expires, persisting
// progress after each trade and the terminal verdict at the end.
func (s *Server) startCampaignMissionRunner(parent context.Context, mission CampaignMission) {
	if mission.Status != CampaignMissionStatusRunning || !time.Now().UTC().Before(mission.ExpiresAt) {
		return
	}
	window := time.Until(mission.ExpiresAt)
	if window <= 0 {
		return
	}
	// A cancelable child of the server runtime context lets Disarm stop this mission
	// without waiting for the window. Persistence writes use `parent` so the terminal
	// state is still recorded after cancellation.
	runCtx, cancel := context.WithCancel(parent)
	if _, loaded := s.campaignRunners.LoadOrStore(mission.ID, cancel); loaded {
		cancel()
		return
	}

	initial := annybasic.State{
		TradesClosed:      mission.TradesClosed,
		ConsecutiveLosses: mission.ConsecutiveLosses,
	}
	if pnl, err := decimal.Parse(defaultString(mission.RealizedPnLUSDT, "0")); err == nil {
		initial.RealizedPnLUSDT = pnl
	} else {
		initial.RealizedPnLUSDT = decimal.Zero()
	}

	advisor := newMissionCampaignAdvisor(
		s.annyBasicObserve,
		json.Number(defaultString(mission.CapitalUSDT, "100")),
		mission.LeverageUsePct,
		missionMaxLeverage,
		initial,
	)

	perTradeClose := campaignPerTradeMaxHold
	if window < perTradeClose {
		perTradeClose = window
	}
	placer := &missionCampaignPlacer{
		orders: s.orders,
		gate:   s.campaignMissionGate(mission.UserKey),
		scheduleClose: func(m timedMission, confirmationID string) error {
			_, err := s.scheduleTimedMissionClose(m, confirmationID)
			return err
		},
		activateClose: s.activateScheduledClose,
		cancelClose:   s.cancelAwaitingScheduledClose,
		nextSeq:       newTradeSeqCounter(mission.LastTradeIdempotencySeq),
		missionID:     mission.ID,
		closeDuration: perTradeClose,
	}

	runner := &missionCampaignRunner{
		advisor:      advisor,
		signals:      campaignexec.NewMarketDataSignals(s.market),
		placer:       placer,
		resolver:     campaignexec.NewRealtimeResolver(s.stream, campaignTradeResolveTimeout),
		goal:         campaignGoalFor(mission),
		symbol:       mission.Symbol,
		userID:       mission.UserID,
		window:       window,
		entryCutoff:  window / campaignEntryCutoffFraction,
		pollInterval: armedMissionCheckInterval,
		logger:       s.logger,
	}

	// Persist progress after each closed trade so a restart resumes from it.
	runner.onRecord = func(state annybasic.State) {
		if _, _, err := s.campaignMissions.UpdateProgress(parent, mission.ID,
			state.TradesClosed, state.RealizedPnLUSDT.String(), state.ConsecutiveLosses,
			int64(state.TradesClosed), time.Now().UTC()); err != nil {
			s.logger.Warn("campaign mission progress persist failed", "id", mission.ID, "error", err)
		}
	}

	go func() {
		defer cancel()
		defer s.campaignRunners.Delete(mission.ID)
		state, verdict, runErr := runner.Run(runCtx)
		status, verdictStr := campaignFinishStatus(verdict, advisor)
		if _, _, err := s.campaignMissions.Finish(parent, mission.ID, status, verdictStr, time.Now().UTC()); err != nil {
			s.logger.Warn("campaign mission finish failed", "id", mission.ID, "error", err)
		}
		s.logger.Info("campaign mission finished",
			"id", mission.ID, "user", mission.UserKey, "symbol", mission.Symbol,
			"status", status, "verdict", verdictStr, "trades", state.TradesClosed,
			"pnl", state.RealizedPnL.String(), "run_err", runErr)
	}()
}

func campaignFinishStatus(verdict campaign.Verdict, advisor *missionCampaignAdvisor) (CampaignMissionStatus, string) {
	switch verdict {
	case campaign.StopTargetReached:
		return CampaignMissionStatusReached, string(verdict)
	case campaign.StopMaxTrades, campaign.StopMaxDrawdown, campaign.StopStrategyRule:
		return CampaignMissionStatusStopped, string(verdict)
	}
	if stopped, reason := advisor.Stopped(); stopped {
		return CampaignMissionStatusStopped, reason
	}
	return CampaignMissionStatusExpired, "window_expired"
}
