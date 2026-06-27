package telegram

import (
	"strings"
	"testing"

	"bottrade/internal/decimal"
	"bottrade/internal/realtime"
)

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
