package campaign

import (
	"testing"

	"bottrade/internal/decimal"
)

func goalFor(target, reward, risk string, winRate, maxTrades int, drawdown string) Goal {
	return Goal{
		CapitalUSDT:        decimal.MustParse("100"),
		TargetProfitUSDT:   decimal.MustParse(target),
		RewardPerTradeUSDT: decimal.MustParse(reward),
		RiskPerTradeUSDT:   decimal.MustParse(risk),
		AssumedWinRate:     winRate,
		MaxTrades:          maxTrades,
		MaxDrawdownUSDT:    decimal.MustParse(drawdown),
	}
}

func TestSimulateReachesTarget(t *testing.T) {
	// 60% win, +2/-1: positive expectancy, should reach +10 and stop as success.
	res := Simulate(goalFor("10", "2", "1", 60, 100, "50"))
	if res.Verdict != StopTargetReached {
		t.Fatalf("verdict = %q, want target_reached", res.Verdict)
	}
	if res.State.RealizedPnL.Cmp(decimal.MustParse("10")) < 0 {
		t.Fatalf("final pnl = %s, want >= 10", res.State.RealizedPnL.String())
	}
	// Win-rate of the simulated sequence should be ~60%.
	wins := 0
	for _, o := range res.Outcomes {
		if o.Win {
			wins++
		}
	}
	got := wins * 100 / len(res.Outcomes)
	if got < 50 || got > 70 {
		t.Fatalf("simulated win-rate = %d%%, want ~60%%", got)
	}
}

func TestSimulateHitsMaxDrawdown(t *testing.T) {
	// 10% win, +2/-1: strongly negative expectancy → drawdown stop.
	res := Simulate(goalFor("100", "2", "1", 10, 0, "10"))
	if res.Verdict != StopMaxDrawdown {
		t.Fatalf("verdict = %q, want max_drawdown", res.Verdict)
	}
	if res.State.RealizedPnL.Cmp(decimal.MustParse("-10").Neg().Neg()) > 0 {
		t.Fatalf("pnl = %s, want <= -10", res.State.RealizedPnL.String())
	}
}

func TestSimulateHitsMaxTrades(t *testing.T) {
	// Break-even-ish with a small trade cap → stop on max trades.
	res := Simulate(goalFor("1000", "1", "1", 50, 8, "1000"))
	if res.Verdict != StopMaxTrades {
		t.Fatalf("verdict = %q, want max_trades", res.Verdict)
	}
	if res.State.TradesClosed != 8 {
		t.Fatalf("trades = %d, want 8", res.State.TradesClosed)
	}
}

func TestSimulateTerminatesOnImpossibleGoal(t *testing.T) {
	// No win-rate, no drawdown stop, no max-trades: must still terminate via the
	// hard cap rather than loop forever.
	res := Simulate(goalFor("1000000", "1", "1", 0, 0, "0"))
	if len(res.Outcomes) == 0 {
		t.Fatal("expected some simulated trades")
	}
	if len(res.Outcomes) > 1000 {
		t.Fatalf("ran %d trades, want capped at 1000", len(res.Outcomes))
	}
}
