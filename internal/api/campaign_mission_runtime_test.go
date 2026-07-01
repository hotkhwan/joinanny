package api

import (
	"context"
	"testing"

	"bottrade/internal/campaign"
	"bottrade/internal/decimal"
	"bottrade/internal/signals"
	"bottrade/internal/strategy/annybasic"
)

func TestCampaignFinishStatusMapping(t *testing.T) {
	stoppedAdvisor := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{})
	// Drive the advisor into a two-loss stop so the "expired vs stopped" branch is real.
	stoppedAdvisor.Record(decimal.NewFromInt(-1))
	stoppedAdvisor.Record(decimal.NewFromInt(-1))
	_, _ = stoppedAdvisor.Decide(context.Background(), signals.MarketSignal{Symbol: "BTCUSDT"})

	fresh := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{})

	tests := []struct {
		name       string
		verdict    campaign.Verdict
		advisor    *missionCampaignAdvisor
		wantStatus CampaignMissionStatus
	}{
		{"target", campaign.StopTargetReached, fresh, CampaignMissionStatusReached},
		{"max trades", campaign.StopMaxTrades, fresh, CampaignMissionStatusStopped},
		{"drawdown", campaign.StopMaxDrawdown, fresh, CampaignMissionStatusStopped},
		{"model stop via cancel", campaign.Continue, stoppedAdvisor, CampaignMissionStatusStopped},
		{"window expired", campaign.Continue, fresh, CampaignMissionStatusExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := campaignFinishStatus(tt.verdict, tt.advisor)
			if got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}

func TestCampaignGoalForDerivesDrawdownFromCapitalRisk(t *testing.T) {
	goal := campaignGoalFor(CampaignMission{
		CapitalUSDT: "100", CapitalRiskPct: 40, MaxTrades: 0,
	})
	if goal.MaxTrades != 15 {
		t.Fatalf("MaxTrades = %d, want default 15", goal.MaxTrades)
	}
	// No user profit target: a sentinel makes the profit-target stop unreachable, so
	// a mission stops only on risk (drawdown / trade cap / window) or the model rule.
	if goal.TargetProfitUSDT.Cmp(decimal.NewFromInt(1_000_000)) < 0 {
		t.Fatalf("target = %s, want a no-stop sentinel (>= 1e6)", goal.TargetProfitUSDT.String())
	}
	// 40% of 100 = 40 drawdown budget.
	if goal.MaxDrawdownUSDT.String() != "40" {
		t.Fatalf("drawdown = %s, want 40", goal.MaxDrawdownUSDT.String())
	}
}
