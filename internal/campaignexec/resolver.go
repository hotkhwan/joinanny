package campaignexec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bottrade/internal/decimal"
	"bottrade/internal/realtime"
)

// EventStream is the slice of *realtime.Broadcaster the resolver needs.
type EventStream interface {
	Subscribe() (<-chan realtime.Event, func())
}

// RealtimeResolver blocks until the realtime position gateway reports the
// symbol's position closed, returning its realized PnL. It implements
// campaign.CloseResolver.
//
// Note: it subscribes when AwaitClose is called (after the order is placed), so a
// position that closes within the poll interval before the subscription starts
// could be missed; on testnet, stop-loss / take-profit fills take long enough
// that this is not a concern in practice. A future user-data WebSocket removes
// the gap entirely.
type RealtimeResolver struct {
	stream  EventStream
	timeout time.Duration
}

// NewRealtimeResolver builds a resolver. timeout bounds the wait for a single
// trade to resolve (default 30m).
func NewRealtimeResolver(stream EventStream, timeout time.Duration) *RealtimeResolver {
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	return &RealtimeResolver{stream: stream, timeout: timeout}
}

// AwaitClose returns the realized PnL of the next close event for symbol.
func (r *RealtimeResolver) AwaitClose(ctx context.Context, symbol string) (decimal.Decimal, error) {
	events, cancel := r.stream.Subscribe()
	defer cancel()

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return decimal.Zero(), ctx.Err()
		case <-timer.C:
			return decimal.Zero(), fmt.Errorf("campaignexec: timed out waiting for %s to close", symbol)
		case event, ok := <-events:
			if !ok {
				return decimal.Zero(), fmt.Errorf("campaignexec: stream closed before %s resolved", symbol)
			}
			if event.Type == realtime.EventTradeClosed && strings.EqualFold(event.Symbol, symbol) {
				return event.RealizedPnL, nil
			}
		}
	}
}
