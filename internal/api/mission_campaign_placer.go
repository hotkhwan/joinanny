package api

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"bottrade/internal/domain"
	"bottrade/internal/orders"
	"bottrade/internal/signals"
)

// missionOrderGateway is the slice of *orders.Service the campaign placer needs.
// It is deliberately narrow so the placer can be unit-tested with a fake and so
// the required-user-executor + idempotency guarantees are visible at the type.
type missionOrderGateway interface {
	PrepareWithIdempotencyKey(ctx context.Context, userID int64, intent domain.Intent, idempotencyKey string) (orders.Confirmation, error)
	ConfirmWithRequiredUserExecutor(ctx context.Context, userID int64, id string) (orders.ExecutionResult, error)
	Cancel(ctx context.Context, userID int64, id string) error
}

// missionCampaignPlacer places each re-entry of a multi-trade mission. It mirrors
// the single-shot armed path's safety ordering (triggerArmedMission) for EVERY
// trade, not just the first:
//   - a distinct, deterministic idempotency key per re-entry ("mission:<id>:trade:<n>")
//     so a retried Place after a crash can never double-open;
//   - the user's own testnet executor only, via ConfirmWithRequiredUserExecutor
//     (never the shared/default executor);
//   - the runtime gate re-asserted before staging AND before confirming;
//   - a durable per-trade timed close scheduled before confirm and armed after,
//     so a crash mid-trade cannot close an unrelated position.
//
// It implements campaign.OrderPlacer.
type missionCampaignPlacer struct {
	orders        missionOrderGateway
	gate          func(ctx context.Context) bool
	scheduleClose func(m timedMission, confirmationID string) error
	activateClose func(ctx context.Context, confirmationID string)
	cancelClose   func(ctx context.Context, confirmationID, reason string)
	nextSeq       func() int64
	missionID     string
	closeDuration time.Duration
}

// Place stages, protects, and confirms one re-entry. It returns the entry
// confirmation id. Any failure leaves nothing live: a staged-but-unconfirmed
// entry is cancelled and its awaiting close removed.
func (p *missionCampaignPlacer) Place(ctx context.Context, userID int64, decision signals.Decision) (string, error) {
	if p.gate != nil && !p.gate(ctx) {
		return "", fmt.Errorf("mission campaign: runtime gate closed; no order staged")
	}
	intent, err := signals.DecisionToIntent(decision, missionMaxLeverage)
	if err != nil {
		return "", fmt.Errorf("mission campaign: decision to intent: %w", err)
	}
	if !intent.IsExchangeChanging() {
		return "", fmt.Errorf("mission campaign: decision did not produce an order intent")
	}
	if intent.Open != nil {
		// Group every trade under the mission id so the journal aggregates the run.
		intent.Open.CampaignID = p.missionID
	}

	key := fmt.Sprintf("mission:%s:trade:%d", p.missionID, p.nextSeq())
	confirmation, err := p.orders.PrepareWithIdempotencyKey(ctx, userID, intent, key)
	if err != nil {
		return "", fmt.Errorf("mission campaign: prepare: %w", err)
	}

	if p.scheduleClose != nil {
		if err := p.scheduleClose(timedMission{UserID: userID, Symbol: decision.Symbol, Side: decision.Side, Duration: p.closeDuration}, confirmation.ID); err != nil {
			_ = p.orders.Cancel(ctx, userID, confirmation.ID)
			return "", fmt.Errorf("mission campaign: schedule close: %w", err)
		}
	}

	if p.gate != nil && !p.gate(ctx) {
		_ = p.orders.Cancel(ctx, userID, confirmation.ID)
		p.dropClose(ctx, confirmation.ID, "gate closed before confirm")
		return "", fmt.Errorf("mission campaign: runtime gate closed before confirm")
	}

	if _, err := p.orders.ConfirmWithRequiredUserExecutor(ctx, userID, confirmation.ID); err != nil {
		p.dropClose(ctx, confirmation.ID, "entry confirm failed: "+err.Error())
		return "", fmt.Errorf("mission campaign: confirm: %w", err)
	}

	// Entry executed → arm the per-trade close.
	if p.activateClose != nil {
		p.activateClose(ctx, confirmation.ID)
	}
	return confirmation.ID, nil
}

func (p *missionCampaignPlacer) dropClose(ctx context.Context, confirmationID, reason string) {
	if p.cancelClose != nil {
		p.cancelClose(ctx, confirmationID, reason)
	}
}

// newTradeSeqCounter returns an in-memory monotonic sequence source starting after
// `start` (0 for a fresh mission, or the persisted LastTradeIdempotencySeq when a
// mission is rehydrated after a restart — see Phase 2). Concurrency-safe, though
// the campaign engine calls Place serially.
func newTradeSeqCounter(start int64) func() int64 {
	n := start
	return func() int64 {
		return atomic.AddInt64(&n, 1)
	}
}
