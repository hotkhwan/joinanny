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
		CapitalUSDT: "100", TargetProfitUSDT: "5", CapitalRiskPct: 40, MaxTrades: 0,
	})
	if goal.MaxTrades != 15 {
		t.Fatalf("MaxTrades = %d, want default 15", goal.MaxTrades)
	}
	if goal.TargetProfitUSDT.String() != "5" {
		t.Fatalf("target = %s, want 5", goal.TargetProfitUSDT.String())
	}
	// 40% of 100 = 40 drawdown budget.
	if goal.MaxDrawdownUSDT.String() != "40" {
		t.Fatalf("drawdown = %s, want 40", goal.MaxDrawdownUSDT.String())
	}
}
