package campaignexec

import (
	"context"
	"time"

	"bottrade/internal/marketdata"
	"bottrade/internal/signals"
)

// PriceSource supplies the current mark price for a symbol.
// *marketdata.BinanceProvider satisfies it via Funding.
type PriceSource interface {
	Funding(ctx context.Context, symbol string) (marketdata.Funding, error)
}

// MarketDataSignals builds the market snapshot the campaign advisor reads from
// the current mark price. The AI's context enricher adds the richer order-flow /
// technical features on top, so a minimal signal is enough here. It implements
// campaign.SignalSource.
type MarketDataSignals struct {
	prices PriceSource
	now    func() time.Time
}

// NewMarketDataSignals wraps a price source.
func NewMarketDataSignals(prices PriceSource) *MarketDataSignals {
	return &MarketDataSignals{prices: prices, now: time.Now}
}

// Signal returns the current market signal for symbol.
func (s *MarketDataSignals) Signal(ctx context.Context, symbol string) (signals.MarketSignal, error) {
	funding, err := s.prices.Funding(ctx, symbol)
	if err != nil {
		return signals.MarketSignal{}, err
	}
	return signals.MarketSignal{
		Source:     "campaign",
		Symbol:     symbol,
		Price:      funding.MarkPrice.String(),
		ReceivedAt: s.now(),
	}, nil
}
