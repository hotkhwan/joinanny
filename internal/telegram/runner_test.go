package telegram

import (
	"context"
	"strings"
	"testing"

	"bottrade/internal/decimal"
	"bottrade/internal/realtime"
)

func TestStartRealtimeIgnoresTypedNilBroadcaster(t *testing.T) {
	// Regression: a nil *realtime.Broadcaster passed into the RealtimeStream
	// interface parameter is a non-nil interface wrapping a nil pointer. It must
	// be treated as no-op, not panic on Subscribe.
	runner := &PollingRunner{logger: testLogger()}
	var nilBroadcaster *realtime.Broadcaster
	runner.StartRealtime(context.Background(), nilBroadcaster, 12345) // must not panic

	// A genuinely nil interface and a zero chat id are also no-ops.
	runner.StartRealtime(context.Background(), nil, 12345)
	runner.StartRealtime(context.Background(), realtime.NewBroadcaster(0), 0)
}

func TestFormatRealtimeAlertOnlyPushesCloses(t *testing.T) {
	// A position update (price tick) must not be pushed to Telegram.
	if _, push := formatRealtimeAlert(realtime.Event{Type: realtime.EventPositionUpdate, Symbol: "BTCUSDT"}); push {
		t.Fatal("position_update should not be pushed to Telegram")
	}

	win := realtime.Event{Type: realtime.EventTradeClosed, Symbol: "BTCUSDT", RealizedPnL: decimal.MustParse("2.5")}
	text, push := formatRealtimeAlert(win)
	if !push || !strings.Contains(text, "🟢") || !strings.Contains(text, "BTCUSDT") || !strings.Contains(text, "2.5") {
		t.Fatalf("win alert = %q (push=%v)", text, push)
	}

	loss := realtime.Event{Type: realtime.EventTradeClosed, Symbol: "ETHUSDT", RealizedPnL: decimal.MustParse("-1.2")}
	text, push = formatRealtimeAlert(loss)
	if !push || !strings.Contains(text, "🔴") || !strings.Contains(text, "-1.2") {
		t.Fatalf("loss alert = %q (push=%v)", text, push)
	}
}

func TestRealtimeBroadcasterSatisfiesStream(t *testing.T) {
	var _ RealtimeStream = realtime.NewBroadcaster(0)
}
