package annybasic

import (
	"math"
	"testing"
	"time"

	"bottrade/internal/marketdata"
)

// TestV13TuningKeepsSetupsReachable locks the v1.3 entry-filter loosening so a
// future edit cannot silently revert to the over-selective v1.2 gating that made
// live armed windows expire without ever finding a setup. See
// docs/model/model-template-and-autotune-spec.md §2.
func TestV13TuningKeepsSetupsReachable(t *testing.T) {
	if cdcFreshResetBars != 0 {
		t.Fatalf("cdcFreshResetBars = %d, want 0 so established trends still qualify", cdcFreshResetBars)
	}
	if qqeFreshCrossBars < signalFreshMainBars {
		t.Fatalf("qqeFreshCrossBars = %d, want >= %d (must not narrow the QQE window)", qqeFreshCrossBars, signalFreshMainBars)
	}
	if sidewaySpreadPct > 0.001 {
		t.Fatalf("sidewaySpreadPct = %v, want <= 0.001 (only block genuinely flat markets)", sidewaySpreadPct)
	}
}

func TestCDCZone(t *testing.T) {
	up := sequence(100, 1, 100)
	if got, ok := cdcZone(up); !ok || got != CDCGreen {
		t.Fatalf("uptrend CDC = %q, %v", got, ok)
	}
	down := sequence(200, -1, 100)
	if got, ok := cdcZone(down); !ok || got != CDCRed {
		t.Fatalf("downtrend CDC = %q, %v", got, ok)
	}
}

func TestQQECanonicalFixture(t *testing.T) {
	values := make([]float64, 140)
	for i := range values {
		values[i] = 100 + 0.08*float64(i) + 3*math.Sin(float64(i)*0.31)
	}
	current, previous, err := qqe(values)
	if err != nil {
		t.Fatalf("qqe: %v", err)
	}
	assertNear(t, current.rsi, 42.897065, 0.000001)
	assertNear(t, current.signal, 53.916597, 0.000001)
	assertNear(t, previous.rsi, 41.818538, 0.000001)
	assertNear(t, previous.signal, 53.916597, 0.000001)
}

func TestObserveAtUsesOnlyClosedMainCandles(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	main := candles(base, 15*time.Minute, sequence(100, 1, 120))
	exec := candles(base.Add(100*15*time.Minute), time.Minute, sequence(200, 0.2, 40))

	_, err := ObserveAt(main, exec, 20)
	if err != nil {
		t.Fatalf("ObserveAt: %v", err)
	}
	closed := closedBefore(main, exec[20].OpenTime)
	for _, candle := range closed {
		if candle.OpenTime.Add(15 * time.Minute).After(exec[20].OpenTime) {
			t.Fatal("adapter included an unclosed 15m candle")
		}
	}
}

func TestCDCTransitionFreshAllowsOneHourConfirmationWindow(t *testing.T) {
	values := make([]float64, 0, 180)
	for i := 0; i < 180; i++ {
		values = append(values, 100+0.03*float64(i)+6*math.Sin(float64(i)*0.18))
	}
	freshIndex := -1
	var zone CDCZone
	for i := 80; i < len(values); i++ {
		gotZone, ok := cdcZone(values[:i])
		if ok && cdcTransitionFresh(values[:i], gotZone, signalFreshMainBars) {
			freshIndex = i
			zone = gotZone
			break
		}
	}
	if freshIndex < 0 {
		t.Fatal("synthetic fixture did not produce a fresh CDC transition")
	}

	aged := append([]float64(nil), values[:freshIndex]...)
	last := aged[len(aged)-1]
	step := 0.15
	if zone == CDCRed {
		step = -0.15
	}
	for i := 0; i < signalFreshMainBars+2; i++ {
		last += step
		aged = append(aged, last)
	}
	agedZone, ok := cdcZone(aged)
	if !ok {
		t.Fatal("aged cdcZone not ready")
	}
	if cdcTransitionFresh(aged, agedZone, signalFreshMainBars) {
		t.Fatal("old transition should expire after the freshness window")
	}
}

func TestRecentQQECrossUsesFreshnessWindow(t *testing.T) {
	values := make([]float64, 0, 180)
	for i := 0; i < 180; i++ {
		values = append(values, 100+0.03*float64(i)+4*math.Sin(float64(i)*0.27))
	}
	crossIndex := -1
	var cross QQECross
	for i := 80; i < len(values); i++ {
		if got := recentQQECross(values[:i], 1); got != QQENone {
			crossIndex = i
			cross = got
			break
		}
	}
	if crossIndex < 0 {
		t.Fatal("synthetic fixture did not produce a QQE cross")
	}
	fresh := append([]float64(nil), values[:crossIndex]...)
	last := fresh[len(fresh)-1]
	for i := 0; i < signalFreshMainBars-1; i++ {
		last += 0.02
		fresh = append(fresh, last)
	}
	if got := recentQQECross(fresh, signalFreshMainBars); got != cross {
		t.Fatalf("fresh QQE cross = %q, want %q", got, cross)
	}
}

func TestEntryExtendedScaledToMainTimeframe(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// 15m main: gentle uptrend with a real per-bar range, so the 15m ATR is a
	// few price units while the 1m execution series below is nearly flat.
	main := make([]marketdata.Candle, 120)
	for i := range main {
		c := 100 + 0.2*float64(i)
		main[i] = marketdata.Candle{
			OpenTime: base.Add(time.Duration(i) * 15 * time.Minute),
			Open:     c - 0.4, High: c + 1.0, Low: c - 1.0, Close: c, Volume: 100,
		}
	}
	fast, ok := emaSeries(candleCloses(main), cdcFastPeriod)
	if !ok {
		t.Fatal("main fast EMA not ready")
	}
	mainFast := fast[len(fast)-1]
	mainATR := averageTrueRange(main, len(main)-1, volatilityPeriod)
	if mainATR <= 0 {
		t.Fatalf("main ATR = %.4f, want > 0", mainATR)
	}

	// Calm 1m execution candles parked at a chosen distance from the 15m fast EMA.
	exec := func(price float64) []marketdata.Candle {
		out := make([]marketdata.Candle, 60)
		start := base.Add(120 * 15 * time.Minute)
		for i := range out {
			out[i] = marketdata.Candle{
				OpenTime: start.Add(time.Duration(i) * time.Minute),
				Open:     price - 0.02, High: price + 0.02, Low: price - 0.02,
				Close: price, Volume: 100,
			}
		}
		return out
	}

	tests := []struct {
		name         string
		price        float64
		wantExtended bool
	}{
		// 1.0x the 15m ATR away: under the old 1m-ATR comparison the tiny 1m ATR
		// flagged this as extended and blocked the window; scaled to the 15m ATR
		// it correctly is not.
		{"within main-timeframe ATR is not extended", mainFast + mainATR, false},
		// 3x the 15m ATR away: a genuinely over-extended entry must still block.
		{"beyond 1.5x main-timeframe ATR is extended", mainFast + 3*mainATR, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candles := exec(tt.price)
			obs, err := ObserveAt(main, candles, len(candles)-1)
			if err != nil {
				t.Fatalf("ObserveAt: %v", err)
			}
			if obs.EntryExtended != tt.wantExtended {
				t.Fatalf("EntryExtended = %v, want %v (deviation vs 1.5x main ATR %.4f)",
					obs.EntryExtended, tt.wantExtended, 1.5*mainATR)
			}
		})
	}
}

func sequence(start, step float64, count int) []float64 {
	out := make([]float64, count)
	for i := range out {
		out[i] = start + step*float64(i)
	}
	return out
}

func candles(start time.Time, interval time.Duration, closes []float64) []marketdata.Candle {
	out := make([]marketdata.Candle, len(closes))
	for i, close := range closes {
		out[i] = marketdata.Candle{
			OpenTime: start.Add(time.Duration(i) * interval),
			Open:     close - 0.1, High: close + 0.2, Low: close - 0.2,
			Close: close, Volume: 100 + float64(i),
		}
	}
	return out
}

func assertNear(t *testing.T, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("got %.9f, want %.9f", got, want)
	}
}
