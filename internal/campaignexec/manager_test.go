package campaignexec

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"bottrade/internal/campaign"
	"bottrade/internal/decimal"
	"bottrade/internal/signals"
)

type holdAdvisor struct{}

func (holdAdvisor) Decide(context.Context, signals.MarketSignal) (signals.Decision, error) {
	return signals.Decision{Action: signals.ActionHold}, nil
}

type staticSignals struct{}

func (staticSignals) Signal(_ context.Context, symbol string) (signals.MarketSignal, error) {
	return signals.MarketSignal{Symbol: symbol, Price: "100"}, nil
}

type nopPlacer struct{}

func (nopPlacer) Place(context.Context, int64, signals.Decision) (string, error) { return "x", nil }

type nopResolver struct{}

func (nopResolver) AwaitClose(context.Context, string) (decimal.Decimal, error) {
	return decimal.Zero(), nil
}

type chanNotifier struct{ ch chan string }

func (n chanNotifier) Notify(text string) { n.ch <- text }

func safeDeps() ManagerDeps {
	return ManagerDeps{
		Signals: staticSignals{}, Advisor: holdAdvisor{},
		Placer: nopPlacer{}, Resolver: nopResolver{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func testGoal() campaign.Goal {
	return campaign.Goal{
		CapitalUSDT: decimal.MustParse("100"), TargetProfitUSDT: decimal.MustParse("10"),
		RewardPerTradeUSDT: decimal.MustParse("2"), RiskPerTradeUSDT: decimal.MustParse("1"),
		AssumedWinRate: 50, MaxTrades: 5, MaxDrawdownUSDT: decimal.MustParse("50"),
	}
}

func TestManagerGatingRefusesUnsafe(t *testing.T) {
	cases := map[string]Safety{
		"disabled":     {Enabled: false, Testnet: true},
		"not testnet":  {Enabled: true, Testnet: false},
		"real trading": {Enabled: true, Testnet: true, RealTradingEnabled: true},
		"dry run":      {Enabled: true, Testnet: true, DryRun: true},
	}
	for name, safety := range cases {
		t.Run(name, func(t *testing.T) {
			m := NewManager(safeDeps(), safety)
			notifier := chanNotifier{ch: make(chan string, 4)}
			if err := m.Start(1, "BTCUSDT", testGoal(), notifier); err == nil {
				t.Fatal("expected the safety gate to refuse")
			}
			if m.IsRunning(1) {
				t.Fatal("nothing should be running after a refused start")
			}
			if len(notifier.ch) != 0 {
				t.Fatal("a refused start must not notify")
			}
		})
	}
}

func TestManagerStartRunsAndFinishes(t *testing.T) {
	// All gates pass; a hold-only advisor makes the engine stop on skip-exhaustion
	// quickly, without placing any order.
	m := NewManager(safeDeps(), Safety{Enabled: true, Testnet: true})
	notifier := chanNotifier{ch: make(chan string, 16)}

	if err := m.Start(1, "BTCUSDT", testGoal(), notifier); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var sawStart, sawEnd bool
	deadline := time.After(3 * time.Second)
	for !sawEnd {
		select {
		case msg := <-notifier.ch:
			if strings.Contains(msg, "started") {
				sawStart = true
			}
			if strings.Contains(msg, "finished") || strings.Contains(msg, "stopped") {
				sawEnd = true
			}
		case <-deadline:
			t.Fatal("campaign did not finish in time")
		}
	}
	if !sawStart {
		t.Fatal("expected a start notification")
	}
	if m.IsRunning(1) {
		t.Fatal("campaign should no longer be running after it finished")
	}
}

func TestManagerStopUnknownUser(t *testing.T) {
	m := NewManager(safeDeps(), Safety{Enabled: true, Testnet: true})
	if m.Stop(999) {
		t.Fatal("stopping a user with no campaign should report false")
	}
}
