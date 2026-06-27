// Package marketdata abstracts the derivatives market-data the AI advisor reads
// (funding, open interest, long/short ratio, taker buy/sell flow) behind a single
// Provider interface, so the data vendor can change without touching the AI.
//
// Phase 1 ships only BinanceProvider, which reads Binance Futures' free public
// endpoints — no API key, and it uses the production base URL even while orders
// execute on testnet, because market data is read-only and unrelated to the
// execution gate. Later phases add Coinalyze / CryptoQuant / CoinGlass providers
// (and can compose several) behind the same interface. MockProvider serves tests
// and offline AI development. See AI_TRADING_SYSTEM.md and the data-source
// roadmap.
package marketdata

import (
	"context"
	"errors"
	"time"

	"bottrade/internal/decimal"
)

// Funding is the current funding rate and mark/index price for a perpetual.
type Funding struct {
	Symbol          string          `json:"symbol"`
	MarkPrice       decimal.Decimal `json:"mark_price"`
	IndexPrice      decimal.Decimal `json:"index_price"`
	LastFundingRate decimal.Decimal `json:"last_funding_rate"`
	NextFundingTime time.Time       `json:"next_funding_time"`
	At              time.Time       `json:"at"`
}

// OpenInterest is the total open interest (in base asset) for a symbol.
type OpenInterest struct {
	Symbol       string          `json:"symbol"`
	OpenInterest decimal.Decimal `json:"open_interest"`
	At           time.Time       `json:"at"`
}

// LongShortRatio is the ratio of accounts (or positions) long versus short. A
// ratio above 1 means more long than short. Period is the sampling window
// ("5m", "1h", ...).
type LongShortRatio struct {
	Symbol       string          `json:"symbol"`
	Period       string          `json:"period"`
	Ratio        decimal.Decimal `json:"ratio"`
	LongAccount  decimal.Decimal `json:"long_account"`
	ShortAccount decimal.Decimal `json:"short_account"`
	At           time.Time       `json:"at"`
}

// TakerFlow is the taker buy/sell volume split over a period. BuySellRatio above
// 1 means aggressive buyers dominated.
type TakerFlow struct {
	Symbol       string          `json:"symbol"`
	Period       string          `json:"period"`
	BuySellRatio decimal.Decimal `json:"buy_sell_ratio"`
	BuyVolume    decimal.Decimal `json:"buy_volume"`
	SellVolume   decimal.Decimal `json:"sell_volume"`
	At           time.Time       `json:"at"`
}

// Ticker is the last traded price and 24h price-change percent for a symbol —
// the lightweight quote the dashboard shows next to favourites.
type Ticker struct {
	Symbol         string          `json:"symbol"`
	LastPrice      decimal.Decimal `json:"last_price"`
	PriceChangePct decimal.Decimal `json:"price_change_pct"`
	At             time.Time       `json:"at"`
}

// Provider reads derivatives market data for one symbol. Implementations are
// expected to be safe for concurrent use.
type Provider interface {
	Funding(ctx context.Context, symbol string) (Funding, error)
	OpenInterest(ctx context.Context, symbol string) (OpenInterest, error)
	LongShortRatio(ctx context.Context, symbol, period string) (LongShortRatio, error)
	TakerFlow(ctx context.Context, symbol, period string) (TakerFlow, error)
}

// Snapshot bundles every metric for one symbol at one moment — the context the
// AI advisor consumes. Fields a provider could not supply are left zero.
type Snapshot struct {
	Symbol       string         `json:"symbol"`
	Period       string         `json:"period"`
	Funding      Funding        `json:"funding"`
	OpenInterest OpenInterest   `json:"open_interest"`
	LongShort    LongShortRatio `json:"long_short"`
	Taker        TakerFlow      `json:"taker"`
	At           time.Time      `json:"at"`
}

// Collect gathers a full Snapshot best-effort: every metric is fetched and any
// that succeed are filled in, so one failing endpoint never blanks the rest. The
// returned error is the join of all per-metric failures (nil when all four
// succeeded); callers may still use the partial snapshot. now stamps the
// snapshot.
func Collect(ctx context.Context, p Provider, symbol, period string, now time.Time) (Snapshot, error) {
	snapshot := Snapshot{Symbol: symbol, Period: period, At: now}
	var errs []error

	if funding, err := p.Funding(ctx, symbol); err != nil {
		errs = append(errs, err)
	} else {
		snapshot.Funding = funding
	}
	if oi, err := p.OpenInterest(ctx, symbol); err != nil {
		errs = append(errs, err)
	} else {
		snapshot.OpenInterest = oi
	}
	if ls, err := p.LongShortRatio(ctx, symbol, period); err != nil {
		errs = append(errs, err)
	} else {
		snapshot.LongShort = ls
	}
	if taker, err := p.TakerFlow(ctx, symbol, period); err != nil {
		errs = append(errs, err)
	} else {
		snapshot.Taker = taker
	}

	return snapshot, errors.Join(errs...)
}

// MockProvider returns canned values for tests and offline AI development. Set
// Err to make every call fail.
type MockProvider struct {
	FundingValue   Funding
	OIValue        OpenInterest
	LongShortValue LongShortRatio
	TakerValue     TakerFlow
	Err            error
}

func (m MockProvider) Funding(context.Context, string) (Funding, error) {
	return m.FundingValue, m.Err
}

func (m MockProvider) OpenInterest(context.Context, string) (OpenInterest, error) {
	return m.OIValue, m.Err
}

func (m MockProvider) LongShortRatio(context.Context, string, string) (LongShortRatio, error) {
	return m.LongShortValue, m.Err
}

func (m MockProvider) TakerFlow(context.Context, string, string) (TakerFlow, error) {
	return m.TakerValue, m.Err
}
