package annybasic

import (
	"fmt"
	"math"
	"time"

	"bottrade/internal/marketdata"
)

const (
	mainInterval        = 15 * time.Minute
	signalFreshMainBars = 4 // legacy 15m freshness base; kept for helper tests / back-compat

	// v1.3 tuning (see docs/model/model-template-and-autotune-spec.md §2). These
	// decouple the two freshness gates that previously shared signalFreshMainBars
	// and made live setups almost never fire inside a short armed window:
	//   - cdcFreshResetBars = 0 disables the "neutralize an established zone" reset,
	//     so a trend that is already green/red still qualifies (enter on the QQE
	//     trigger inside the trend, not only in the first hour after a flip).
	//   - qqeFreshCrossBars widens the QQE cross window to 8 bars (2h).
	//   - sidewaySpreadPct only blocks genuinely flat markets (0.05% of price).
	cdcFreshResetBars = 0
	qqeFreshCrossBars = 8
	sidewaySpreadPct  = 0.0005
)

// ObserveAt builds a closed-candle 15m CDC/QQE observation and confirms it
// against 1m execution candles through executionIndex.
func ObserveAt(main15m, execution1m []marketdata.Candle, executionIndex int) (Observation, error) {
	if executionIndex < 0 || executionIndex >= len(execution1m) {
		return Observation{}, fmt.Errorf("anny basic: execution index out of range")
	}
	at := execution1m[executionIndex].OpenTime
	main := closedBefore(main15m, at)
	if len(main) == 0 {
		return Observation{}, errInsufficientData
	}
	mainCloses := candleCloses(main)
	zone, ok := cdcZone(mainCloses)
	if !ok {
		return Observation{}, errInsufficientData
	}
	// v1.3: only neutralize an aged trend when cdcFreshResetBars > 0. With the
	// reset disabled (default), an established green/red zone still qualifies, so
	// entries fire on a fresh QQE trigger inside the trend rather than only in the
	// first hour after a CDC flip.
	if cdcFreshResetBars > 0 && !cdcTransitionFresh(mainCloses, zone, cdcFreshResetBars) {
		zone = CDCNeutral
	}
	currentQQE, _, err := qqe(mainCloses)
	if err != nil {
		return Observation{}, err
	}
	cross := recentQQECross(mainCloses, qqeFreshCrossBars)

	exec := execution1m[:executionIndex+1]
	execCloses := candleCloses(exec)
	fast, fastOK := emaSeries(execCloses, execFastPeriod)
	slow, slowOK := emaSeries(execCloses, execSlowPeriod)
	if !fastOK || !slowOK || len(exec) < volumePeriod+1 {
		return Observation{}, errInsufficientData
	}
	aligned := (zone == CDCGreen && fast[len(fast)-1] > slow[len(slow)-1]) ||
		(zone == CDCRed && fast[len(fast)-1] < slow[len(slow)-1])

	atr := averageTrueRange(exec, executionIndex, volatilityPeriod)
	last := exec[executionIndex]
	avgVolume := averageVolume(exec, executionIndex, volumePeriod)
	body := math.Abs(last.Close - last.Open)
	mainFast, _ := emaSeries(mainCloses, cdcFastPeriod)
	// EntryExtended measures how far price has run from the 15m fast EMA, so it
	// must be compared against a 15m-scale ATR. Comparing that 15m deviation
	// against the 1m ATR (roughly 10-15x smaller) made almost every bar read as
	// extended, so the market-condition gate blocked whole validation windows and
	// no setup was ever launchable.
	mainATR := averageTrueRange(main, len(main)-1, volatilityPeriod)
	extended := mainATR > 0 && math.Abs(last.Close-mainFast[len(mainFast)-1]) > 1.5*mainATR
	abnormal := atr > 0 && trueRange(exec, executionIndex) > 3*atr
	sideways := last.Close > 0 && cdcSpread(mainCloses)/last.Close < sidewaySpreadPct

	return Observation{
		CDC15m:             zone,
		QQEValue:           currentQQE.rsi,
		QQECross:           cross,
		ExecutionAligned:   aligned,
		MomentumConfirmed:  avgVolume > 0 && last.Volume > avgVolume && body >= atr,
		EntryExtended:      extended,
		AbnormalVolatility: abnormal,
		Sideways:           sideways,
	}, nil
}

func cdcTransitionFresh(closes []float64, current CDCZone, maxBars int) bool {
	if current == CDCNeutral || maxBars <= 0 {
		return false
	}
	if len(closes) < 2 {
		return false
	}
	limit := maxBars
	if len(closes)-1 < limit {
		limit = len(closes) - 1
	}
	for barsAgo := 1; barsAgo <= limit; barsAgo++ {
		previous, ok := cdcZone(closes[:len(closes)-barsAgo])
		if !ok {
			return false
		}
		if previous != current {
			return true
		}
	}
	return false
}

func recentQQECross(closes []float64, maxBars int) QQECross {
	if maxBars <= 0 {
		return QQENone
	}
	limit := maxBars
	if len(closes) < limit+1 {
		limit = len(closes) - 1
	}
	for barsAgo := 0; barsAgo < limit; barsAgo++ {
		end := len(closes) - barsAgo
		current, previous, err := qqe(closes[:end])
		if err != nil {
			return QQENone
		}
		switch {
		case previous.rsi <= previous.signal && current.rsi > current.signal:
			return QQECrossUp
		case previous.rsi >= previous.signal && current.rsi < current.signal:
			return QQECrossDown
		}
	}
	return QQENone
}

func closedBefore(candles []marketdata.Candle, at time.Time) []marketdata.Candle {
	end := 0
	for i, candle := range candles {
		if candle.OpenTime.Add(mainInterval).After(at) {
			break
		}
		end = i + 1
	}
	return candles[:end]
}

func candleCloses(candles []marketdata.Candle) []float64 {
	out := make([]float64, len(candles))
	for i, candle := range candles {
		out[i] = candle.Close
	}
	return out
}

func cdcSpread(closes []float64) float64 {
	fast, _ := emaSeries(closes, cdcFastPeriod)
	slow, _ := emaSeries(closes, cdcSlowPeriod)
	return math.Abs(fast[len(fast)-1] - slow[len(slow)-1])
}

func averageTrueRange(candles []marketdata.Candle, index, period int) float64 {
	start := index - period + 1
	if start < 1 {
		start = 1
	}
	var total float64
	var count int
	for i := start; i <= index; i++ {
		total += trueRange(candles, i)
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func trueRange(candles []marketdata.Candle, index int) float64 {
	value := candles[index].High - candles[index].Low
	if index == 0 {
		return value
	}
	value = math.Max(value, math.Abs(candles[index].High-candles[index-1].Close))
	return math.Max(value, math.Abs(candles[index].Low-candles[index-1].Close))
}

func averageVolume(candles []marketdata.Candle, index, period int) float64 {
	start := index - period
	if start < 0 {
		start = 0
	}
	if start == index {
		return 0
	}
	var total float64
	for _, candle := range candles[start:index] {
		total += candle.Volume
	}
	return total / float64(index-start)
}
