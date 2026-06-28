// Package campaignexec wires the autonomous campaign engine to live execution:
// a placer that submits a decision through the order service (on the user's own
// per-user executor) and a resolver that waits on the realtime position gateway
// for the trade to close. Both are deliberately thin and interface-driven so the
// risky autonomous path stays unit-testable and the safety gates live in one
// place (the campaign command's preconditions).
package campaignexec

import (
	"context"
	"fmt"

	"bottrade/internal/domain"
	"bottrade/internal/orders"
	"bottrade/internal/signals"
)

// OrderService is the slice of *orders.Service the placer needs.
type OrderService interface {
	Prepare(ctx context.Context, userID int64, intent domain.Intent) (orders.Confirmation, error)
	Confirm(ctx context.Context, userID int64, id string) (orders.ExecutionResult, error)
}

// ServicePlacer submits a campaign decision as a real order by preparing and
// immediately confirming it — the autonomous equivalent of the human
// [Confirm] press. It implements campaign.OrderPlacer.
type ServicePlacer struct {
	service     OrderService
	maxLeverage int
}

// NewServicePlacer builds a placer bound to the order service.
func NewServicePlacer(service OrderService, maxLeverage int) *ServicePlacer {
	if maxLeverage <= 0 {
		maxLeverage = 20
	}
	return &ServicePlacer{service: service, maxLeverage: maxLeverage}
}

// Place turns the decision into an intent, prepares it, and confirms it. The
// order goes out on the user's own executor (testnet, gated upstream). It
// returns the entry order's client id.
func (p *ServicePlacer) Place(ctx context.Context, userID int64, decision signals.Decision) (string, error) {
	intent, err := signals.DecisionToIntent(decision, p.maxLeverage)
	if err != nil {
		return "", fmt.Errorf("campaignexec: decision to intent: %w", err)
	}
	if !intent.IsExchangeChanging() {
		return "", fmt.Errorf("campaignexec: decision did not produce an order intent")
	}

	confirmation, err := p.service.Prepare(ctx, userID, intent)
	if err != nil {
		return "", fmt.Errorf("campaignexec: prepare: %w", err)
	}
	result, err := p.service.Confirm(ctx, userID, confirmation.ID)
	if err != nil {
		return "", fmt.Errorf("campaignexec: confirm: %w", err)
	}
	return result.ClientOrderID, nil
}
