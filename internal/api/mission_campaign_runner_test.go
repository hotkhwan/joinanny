package api

import (
	"context"
	"testing"
	"time"

	"bottrade/internal/campaign"
	"bottrade/internal/decimal"
	"bottrade/internal/domain"
	"bottrade/internal/orders"
	"bottrade/internal/signals"
	"bottrade/internal/strategy/annybasic"
)

// --- fakes ---------------------------------------------------------------

type fakeMissionGateway struct {
	prepareKeys []string
	confirms    int
	cancels     int
	confirmErr  error
	prepareErr  error
}

func (g *fakeMissionGateway) PrepareWithIdempotencyKey(_ context.Context, _ int64, _ domain.Intent, key string) (orders.Confirmation, error) {
	if g.prepareErr != nil {
		return orders.Confirmation{}, g.prepareErr
	}
	g.prepareKeys = append(g.prepareKeys, key)
	return orders.Confirmation{ID: "conf-" + key}, nil
}

func (g *fakeMissionGateway) ConfirmWithRequiredUserExecutor(_ context.Context, _ int64, id string) (orders.ExecutionResult, error) {
	g.confirms++
	if g.confirmErr != nil {
		return orders.ExecutionResult{}, g.confirmErr
	}
	return orders.ExecutionResult{ClientOrderID: id}, nil
}

func (g *fakeMissionGateway) Cancel(_ context.Context, _ int64, _ string) error {
	g.cancels++
	return nil
}

type fakeSignals struct{ price string }

func (f *fakeSignals) Signal(_ context.Context, symbol string) (signals.MarketSignal, error) {
	return signals.MarketSignal{Symbol: symbol, Price: f.price}, nil
}

type fakePlacer struct{ calls int }

func (f *fakePlacer) Place(_ context.Context, _ int64, _ signals.Decision) (string, error) {
	f.calls++
	return "ref", nil
}

type fakeResolver struct{ pnl decimal.Decimal }

func (f *fakeResolver) AwaitClose(_ context.Context, _ string) (decimal.Decimal, error) {
	return f.pnl, nil
}

// --- placer tests --------------------------------------------------------

func newTestPlacer(gw missionOrderGateway, gate func(context.Context) bool) (*missionCampaignPlacer, *int, *int) {
	scheduled, activated := 0, 0
	p := &missionCampaignPlacer{
		orders:        gw,
		gate:          gate,
		scheduleClose: func(timedMission, string) error { scheduled++; return nil },
		activateClose: func(context.Context, string) { activated++ },
		cancelClose:   func(context.Context, string, string) {},
		nextSeq:       newTradeSeqCounter(0),
		missionID:     "abc",
		closeDuration: time.Hour,
	}
	return p, &scheduled, &activated
}

func validOpenDecision(t *testing.T) signals.Decision {
	t.Helper()
	a := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{})
	return decideBTC(t, a)
}

func TestMissionCampaignPlacerIdempotencyKeyPerTrade(t *testing.T) {
	gw := &fakeMissionGateway{}
	p, scheduled, activated := newTestPlacer(gw, func(context.Context) bool { return true })
	decision := validOpenDecision(t)

	ref1, err := p.Place(context.Background(), 7, decision)
	if err != nil {
		t.Fatalf("Place 1: %v", err)
	}
	ref2, err := p.Place(context.Background(), 7, decision)
	if err != nil {
		t.Fatalf("Place 2: %v", err)
	}

	if want := []string{"mission:abc:trade:1", "mission:abc:trade:2"}; gw.prepareKeys[0] != want[0] || gw.prepareKeys[1] != want[1] {
		t.Fatalf("idempotency keys = %v, want %v", gw.prepareKeys, want)
	}
	if ref1 != "conf-mission:abc:trade:1" || ref2 != "conf-mission:abc:trade:2" {
		t.Fatalf("refs = %q,%q", ref1, ref2)
	}
	if gw.confirms != 2 || *scheduled != 2 || *activated != 2 {
		t.Fatalf("confirms=%d scheduled=%d activated=%d, want 2/2/2", gw.confirms, *scheduled, *activated)
	}
}

func TestMissionCampaignPlacerGateClosedBeforeConfirmCancels(t *testing.T) {
	gw := &fakeMissionGateway{}
	calls := 0
	// Gate open at the pre-stage check, closed at the pre-confirm check.
	gate := func(context.Context) bool { calls++; return calls == 1 }
	p, _, activated := newTestPlacer(gw, gate)

	_, err := p.Place(context.Background(), 7, validOpenDecision(t))
	if err == nil {
		t.Fatal("expected error when gate closes before confirm")
	}
	if gw.confirms != 0 {
		t.Fatalf("confirms = %d, want 0 (must not confirm through a closed gate)", gw.confirms)
	}
	if gw.cancels != 1 {
		t.Fatalf("cancels = %d, want 1 (staged entry must be cancelled)", gw.cancels)
	}
	if *activated != 0 {
		t.Fatalf("activated = %d, want 0", *activated)
	}
}

func TestMissionCampaignPlacerConfirmFailureCancelsClose(t *testing.T) {
	gw := &fakeMissionGateway{confirmErr: context.DeadlineExceeded}
	cancelledClose := 0
	p := &missionCampaignPlacer{
		orders:        gw,
		gate:          func(context.Context) bool { return true },
		scheduleClose: func(timedMission, string) error { return nil },
		activateClose: func(context.Context, string) {},
		cancelClose:   func(context.Context, string, string) { cancelledClose++ },
		nextSeq:       newTradeSeqCounter(0),
		missionID:     "abc",
		closeDuration: time.Hour,
	}
	if _, err := p.Place(context.Background(), 7, validOpenDecision(t)); err == nil {
		t.Fatal("expected confirm failure to surface")
	}
	if cancelledClose != 1 {
		t.Fatalf("cancelledClose = %d, want 1 (awaiting close must be dropped on confirm failure)", cancelledClose)
	}
}

// --- runner tests --------------------------------------------------------

func newTestRunner(a *missionCampaignAdvisor, resolverPnL decimal.Decimal, goal campaign.Goal) *missionCampaignRunner {
	fixed := time.Unix(1710000000, 0)
	return &missionCampaignRunner{
		advisor:      a,
		signals:      &fakeSignals{price: "60000"},
		placer:       &fakePlacer{},
		resolver:     &fakeResolver{pnl: resolverPnL},
		goal:         goal,
		symbol:       "BTCUSDT",
		userID:       7,
		window:       time.Hour,
		entryCutoff:  time.Minute,
		pollInterval: time.Millisecond,
		now:          func() time.Time { return fixed },
		sleep:        func(context.Context, time.Duration) error { return nil },
	}
}

func TestMissionCampaignRunnerStopsOnGoalTarget(t *testing.T) {
	a := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{RealizedPnLUSDT: decimal.Zero()})
	r := newTestRunner(a, decimal.NewFromInt(5), campaign.Goal{TargetProfitUSDT: decimal.NewFromInt(10), MaxTrades: 15})

	state, verdict, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if verdict != campaign.StopTargetReached {
		t.Fatalf("verdict = %q, want target_reached", verdict)
	}
	if state.TradesClosed != 2 {
		t.Fatalf("TradesClosed = %d, want 2 (two +5 trades reach the 10 target)", state.TradesClosed)
	}
	if a.State().TradesClosed != 2 {
		t.Fatalf("advisor recorded %d trades, want 2 (state feedback broken)", a.State().TradesClosed)
	}
}

func TestMissionCampaignRunnerStopsOnModelTwoLosses(t *testing.T) {
	// High Goal target so only the model's two-consecutive-loss rule can end it —
	// exercising the onStop cancel path (Engine skip guard is disabled).
	a := newMissionCampaignAdvisor(observeStub(longSetup(), 60000), "100", 50, 100, annybasic.State{RealizedPnLUSDT: decimal.Zero()})
	r := newTestRunner(a, decimal.NewFromInt(-1), campaign.Goal{TargetProfitUSDT: decimal.NewFromInt(1000), MaxTrades: 15, MaxDrawdownUSDT: decimal.NewFromInt(1000)})

	state, _, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected the run to end via cancelled context on the two-loss stop")
	}
	if state.TradesClosed != 2 {
		t.Fatalf("TradesClosed = %d, want 2 (stop after two consecutive losses)", state.TradesClosed)
	}
	if stopped, reason := a.Stopped(); !stopped || reason != "two consecutive losses" {
		t.Fatalf("advisor Stopped = (%v,%q), want two consecutive losses", stopped, reason)
	}
}
