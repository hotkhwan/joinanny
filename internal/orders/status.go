package orders

import (
	"context"
	"fmt"
	"strings"

	"bottrade/internal/domain"
)

type PositionProvider interface {
	Positions(ctx context.Context) ([]domain.Position, error)
}

type EmptyPositionProvider struct{}

func (EmptyPositionProvider) Positions(ctx context.Context) ([]domain.Position, error) {
	return nil, nil
}

type StatusService struct {
	provider PositionProvider
}

type StatusSnapshot struct {
	Text      string
	Positions []domain.Position
	Err       error
}

func NewStatusService(provider PositionProvider) *StatusService {
	if provider == nil {
		provider = EmptyPositionProvider{}
	}
	return &StatusService{provider: provider}
}

func (s *StatusService) Snapshot(ctx context.Context) StatusSnapshot {
	positions, err := s.provider.Positions(ctx)
	return StatusSnapshot{
		Text:      formatStatusText(positions, err),
		Positions: positions,
		Err:       err,
	}
}

func (s *StatusService) Text(ctx context.Context) string {
	return s.Snapshot(ctx).Text
}

func formatStatusText(positions []domain.Position, err error) string {
	if err != nil {
		return "Status lookup failed: " + err.Error()
	}
	if len(positions) == 0 {
		return "No open positions."
	}

	var builder strings.Builder
	builder.WriteString("Open positions:\n\n")
	for i, position := range positions {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(formatPosition(position))
	}
	return builder.String()
}

func formatPosition(position domain.Position) string {
	return fmt.Sprintf(
		"%s %s\nQty: %s\nEntry: %s\nMark: %s\nUnrealized PnL: %s USDT\nLeverage: %dx\nMargin: %s",
		strings.ToUpper(string(position.Side)),
		position.Symbol,
		position.Amount.String(),
		position.EntryPrice.String(),
		position.MarkPrice.String(),
		position.UnrealizedProfit.String(),
		position.Leverage,
		position.MarginType,
	)
}
