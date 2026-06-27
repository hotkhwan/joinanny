package campaignexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"bottrade/internal/decimal"
	"bottrade/internal/domain"
	"bottrade/internal/orders"
	"bottrade/internal/realtime"
	"bottrade/internal/signals"
)

type fakeOrderService struct {
	prepared, confirmed bool
	prepareErr          error
}

func (f *fakeOrderService) Prepare(context.Context, int64, domain.Intent) (orders.Confirmation, error) {
	if f.prepareErr != nil {
		return orders.Confirmation{}, f.prepareErr
	}
	f.prepared = true
	return orders.Confirmation{ID: "conf-1"}, nil
}

func (f *fakeOrderService) Confirm(context.Context, int64, string) (orders.ExecutionResult, error) {
	f.confirmed = true
	return orders.ExecutionResult{ClientOrderID: "tb_entry"}, nil
}

func openDecision() signals.Decision {
	return signals.Decision{
		Action: signals.ActionOpen, Symbol: "BTCUSDT", Side: "long", Leverage: 3,
		Entry: "67500", StopLoss: "65000", TakeProfit: "72000", SizeUSDT: "100",
	}
}

func TestServicePlacerPreparesAndConfirms(t *testing.T) {
	svc := &fakeOrderService{}
	ref, err := NewServicePlacer(svc, 20).Place(context.Background(), 7, openDecision())
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if ref != "tb_entry" {
		t.Fatalf("ref = %q, want tb_entry", ref)
	}
	if !svc.prepared || !svc.confirmed {
		t.Fatalf("expected prepare+confirm, got prepared=%v confirmed=%v", svc.prepared, svc.confirmed)
	}
}

func TestServicePlacerRejectsBadDecision(t *testing.T) {
	// A hold decision does not produce an order intent.
	_, err := NewServicePlacer(&fakeOrderService{}, 20).
		Place(context.Background(), 7, signals.Decision{Action: signals.ActionHold, Symbol: "BTCUSDT"})
	if err == nil {
		t.Fatal("expected an error for a non-order decision")
	}
}

func TestServicePlacerPropagatesPrepareError(t *testing.T) {
	svc := &fakeOrderService{prepareErr: errors.New("no credential")}
	if _, err := NewServicePlacer(svc, 20).Place(context.Background(), 7, openDecision()); err == nil {
		t.Fatal("expected prepare error to propagate")
	}
}

func TestRealtimeResolverReturnsRealizedPnL(t *testing.T) {
	broadcaster := realtime.NewBroadcaster(8)
	resolver := NewRealtimeResolver(broadcaster, 2*time.Second)

	done := make(chan decimal.Decimal, 1)
	go func() {
		pnl, err := resolver.AwaitClose(context.Background(), "BTCUSDT")
		if err != nil {
			t.Errorf("AwaitClose: %v", err)
		}
		done <- pnl
	}()

	// Publish until the goroutine has subscribed and consumed an event. An
	// unrelated symbol must be ignored.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case pnl := <-done:
			if pnl.String() != "2.5" {
				t.Fatalf("pnl = %s, want 2.5", pnl.String())
			}
			return
		case <-deadline:
			t.Fatal("resolver did not return in time")
		default:
			broadcaster.Publish(realtime.Event{Type: realtime.EventPositionUpdate, Symbol: "BTCUSDT"})
			broadcaster.Publish(realtime.Event{Type: realtime.EventTradeClosed, Symbol: "ETHUSDT", RealizedPnL: decimal.MustParse("9")})
			broadcaster.Publish(realtime.Event{Type: realtime.EventTradeClosed, Symbol: "BTCUSDT", RealizedPnL: decimal.MustParse("2.5")})
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestRealtimeResolverTimesOut(t *testing.T) {
	resolver := NewRealtimeResolver(realtime.NewBroadcaster(8), 40*time.Millisecond)
	if _, err := resolver.AwaitClose(context.Background(), "BTCUSDT"); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestRealtimeResolverContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := NewRealtimeResolver(realtime.NewBroadcaster(8), time.Minute)
	if _, err := resolver.AwaitClose(ctx, "BTCUSDT"); err == nil {
		t.Fatal("expected context cancellation error")
	}
}
