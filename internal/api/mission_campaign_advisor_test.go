package api

import (
	"context"
	"strconv"
	"testing"

	"bottrade/internal/decimal"
	"bottrade/internal/signals"
	"bottrade/internal/strategy/annybasic"
)

func priceF(t *testing.T, s string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parse price %q: %v", s, err)
	}
	return f
}

func longSetup() annybasic.Observation {
	return annybasic.Observation{
		CDC15m: annybasic.CDCGreen, QQEValue: 61, QQECross: annybasic.QQECrossUp,
		ExecutionAligned: true, MomentumConfirmed: true,
	}
}

func shortSetup() annybasic.Observation {
	return annybasic.Observation{
		CDC15m: annybasic.CDCRed, QQEValue: 39, QQECross: annybasic.QQECrossDown,
		ExecutionAligned: true,
	}
}

func observeStub(obs annybasic.Observation, price float64) annyBasicObserveFunc {
	return func(context.Context, string) (annybasic.Observation, float64, error) {
		return obs, price, nil
	}
}

func decideBTC(t *testing.T, a *missionCampaignAdvisor) signals.Decision {
	t.Helper()
	d, err := a.Decide(context.Background(), signals.MarketSignal{Symbol: "BTCUSDT"})
	if err != nil {
		t.Fatalf("Decide error: %v", err)
	}
	return d
}

func TestMissionCampaignAdvisorOpensCappedLong(t *testing.T) {
	a := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{RealizedPnLUSDT: decimal.Zero()})
	d := decideBTC(t, a)

	if d.Action != signals.ActionOpen || d.Side != "long" {
		t.Fatalf("decision = %+v, want ActionOpen long", d)
	}
	if d.Leverage <= 0 || d.Leverage > missionMaxLeverage {
		t.Fatalf("leverage = %d, want (0, %d]", d.Leverage, missionMaxLeverage)
	}
	if d.SizeUSDT == "" || d.Entry == "" || d.StopLoss == "" || d.TakeProfit == "" {
		t.Fatalf("decision missing sizing/bracket: %+v", d)
	}
	// Long bracket: stop below entry, take-profit above.
	if !(priceF(t, d.StopLoss) < priceF(t, d.Entry) && priceF(t, d.Entry) < priceF(t, d.TakeProfit)) {
		t.Fatalf("long bracket SL=%s Entry=%s TP=%s, want SL < Entry < TP", d.StopLoss, d.Entry, d.TakeProfit)
	}
}

func TestMissionCampaignAdvisorShortBracketInverted(t *testing.T) {
	a := newMissionCampaignAdvisor(observeStub(shortSetup(), 60000), "100", 50, 100, annybasic.State{})
	d := decideBTC(t, a)
	if d.Action != signals.ActionOpen || d.Side != "short" {
		t.Fatalf("decision = %+v, want ActionOpen short", d)
	}
	// Short bracket: stop above entry, take-profit below.
	if !(priceF(t, d.TakeProfit) < priceF(t, d.Entry) && priceF(t, d.Entry) < priceF(t, d.StopLoss)) {
		t.Fatalf("short bracket SL=%s Entry=%s TP=%s, want TP < Entry < SL", d.StopLoss, d.Entry, d.TakeProfit)
	}
}

func TestMissionCampaignAdvisorHoldsWithoutSetup(t *testing.T) {
	noSetup := annybasic.Observation{CDC15m: annybasic.CDCGreen, QQEValue: 49, QQECross: annybasic.QQECrossDown, ExecutionAligned: true}
	a := newMissionCampaignAdvisor(observeStub(noSetup, 60000), "100", 50, 100, annybasic.State{})
	d := decideBTC(t, a)
	if d.Action != signals.ActionHold {
		t.Fatalf("decision = %+v, want ActionHold when CDC/QQE not aligned", d)
	}
	if stopped, _ := a.Stopped(); stopped {
		t.Fatal("advisor stopped without a campaign-stop condition")
	}
}

// The core Phase-0 guarantee: accumulated State makes the model's campaign-stop
// rules fire live, unlike the single-shot armed path which evaluates against a
// zero State. Each case records outcomes, then Decide must hold + report stopped.
func TestMissionCampaignAdvisorStopRulesFireFromAccumulatedState(t *testing.T) {
	tests := []struct {
		name       string
		record     func(a *missionCampaignAdvisor)
		wantReason string
	}{
		{
			name:       "profit target reached",
			record:     func(a *missionCampaignAdvisor) { a.Record(decimal.NewFromInt(10)) },
			wantReason: "profit target reached",
		},
		{
			name: "two consecutive losses",
			record: func(a *missionCampaignAdvisor) {
				a.Record(decimal.NewFromInt(-1))
				a.Record(decimal.NewFromInt(-1))
			},
			wantReason: "two consecutive losses",
		},
		{
			name: "trade cap",
			record: func(a *missionCampaignAdvisor) {
				// 15 breakeven-positive trades: below target, never two losses in a row.
				for i := 0; i < 15; i++ {
					a.Record(decimal.Zero())
				}
			},
			wantReason: "15-trade hard stop",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{RealizedPnLUSDT: decimal.Zero()})
			tt.record(a)
			d := decideBTC(t, a)
			if d.Action != signals.ActionHold {
				t.Fatalf("decision = %+v, want ActionHold once stop rule fires", d)
			}
			stopped, reason := a.Stopped()
			if !stopped || reason != tt.wantReason {
				t.Fatalf("Stopped() = (%v, %q), want (true, %q)", stopped, reason, tt.wantReason)
			}
		})
	}
}

func TestMissionCampaignAdvisorRecordTracksLossStreakAndReset(t *testing.T) {
	a := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{RealizedPnLUSDT: decimal.Zero()})
	a.Record(decimal.NewFromInt(-1))
	if got := a.State().ConsecutiveLosses; got != 1 {
		t.Fatalf("ConsecutiveLosses after one loss = %d, want 1", got)
	}
	a.Record(decimal.NewFromInt(2)) // a win resets the streak
	st := a.State()
	if st.ConsecutiveLosses != 0 {
		t.Fatalf("ConsecutiveLosses after win = %d, want 0", st.ConsecutiveLosses)
	}
	if st.TradesClosed != 2 {
		t.Fatalf("TradesClosed = %d, want 2", st.TradesClosed)
	}
	if st.RealizedPnLUSDT.Cmp(decimal.NewFromInt(1)) != 0 {
		t.Fatalf("RealizedPnL = %s, want 1", st.RealizedPnLUSDT.String())
	}
}
