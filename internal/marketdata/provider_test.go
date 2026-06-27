package marketdata

import (
	"context"
	"errors"
	"testing"
	"time"

	"bottrade/internal/decimal"
)

func testTime() time.Time { return time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC) }

func TestCollectWithMockProvider(t *testing.T) {
	mock := MockProvider{
		FundingValue:   Funding{Symbol: "BTCUSDT", LastFundingRate: decimal.MustParse("0.0001")},
		OIValue:        OpenInterest{Symbol: "BTCUSDT", OpenInterest: decimal.MustParse("100")},
		LongShortValue: LongShortRatio{Symbol: "BTCUSDT", Ratio: decimal.MustParse("1.5")},
		TakerValue:     TakerFlow{Symbol: "BTCUSDT", BuySellRatio: decimal.MustParse("1.1")},
	}
	snap, err := Collect(context.Background(), mock, "BTCUSDT", "5m", testTime())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !snap.At.Equal(testTime()) || snap.Symbol != "BTCUSDT" || snap.Period != "5m" {
		t.Fatalf("snapshot meta = %+v", snap)
	}
	if snap.Funding.LastFundingRate.String() != "0.0001" || snap.LongShort.Ratio.String() != "1.5" {
		t.Fatalf("snapshot values = %+v", snap)
	}
}

func TestCollectJoinsErrorsButKeepsPartial(t *testing.T) {
	// Every metric fails: Collect returns a joined error but still a usable
	// (zero-valued) snapshot rather than blanking everything on first failure.
	mock := MockProvider{Err: errors.New("boom")}
	snap, err := Collect(context.Background(), mock, "BTCUSDT", "5m", testTime())
	if err == nil {
		t.Fatal("expected joined error when all metrics fail")
	}
	if snap.Symbol != "BTCUSDT" {
		t.Fatalf("snapshot should still carry meta: %+v", snap)
	}
}

func TestMockProviderSatisfiesProvider(t *testing.T) {
	var _ Provider = MockProvider{}
}

func TestBinanceProviderSatisfiesProvider(t *testing.T) {
	var _ Provider = NewBinanceProvider("", nil)
}
