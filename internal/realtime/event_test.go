package realtime

import (
	"testing"
	"time"
)

func TestParseUserDataEventClose(t *testing.T) {
	raw := []byte(`{"e":"ORDER_TRADE_UPDATE","E":1700000000000,"o":{"s":"BTCUSDT","S":"SELL","x":"TRADE","X":"FILLED","rp":"2.5","ap":"68000","R":true}}`)
	event, ok, err := ParseUserDataEvent(raw, time.Now())
	if err != nil || !ok {
		t.Fatalf("ParseUserDataEvent = (ok=%v, err=%v), want a parsed event", ok, err)
	}
	if event.Type != EventTradeClosed {
		t.Fatalf("type = %q, want trade_closed", event.Type)
	}
	if event.RealizedPnL.String() != "2.5" || event.Symbol != "BTCUSDT" || event.Side != "sell" {
		t.Fatalf("event = %+v, want BTCUSDT sell pnl 2.5", event)
	}
	if !event.At.Equal(time.UnixMilli(1700000000000).UTC()) {
		t.Fatalf("at = %v, want the event time", event.At)
	}
}

func TestParseUserDataEventOpeningFillIsUpdate(t *testing.T) {
	raw := []byte(`{"e":"ORDER_TRADE_UPDATE","o":{"s":"ETHUSDT","S":"BUY","x":"TRADE","X":"FILLED","rp":"0","ap":"3000","R":false}}`)
	event, ok, err := ParseUserDataEvent(raw, time.Unix(10, 0).UTC())
	if err != nil || !ok {
		t.Fatalf("ParseUserDataEvent = (ok=%v, err=%v)", ok, err)
	}
	if event.Type != EventPositionUpdate {
		t.Fatalf("type = %q, want position_update for an opening fill", event.Type)
	}
}

func TestParseUserDataEventIgnoresNonFills(t *testing.T) {
	for _, raw := range []string{
		`{"e":"ACCOUNT_UPDATE","a":{}}`,
		`{"e":"ORDER_TRADE_UPDATE","o":{"s":"BTCUSDT","x":"NEW","X":"NEW"}}`,
		`{"e":"listenKeyExpired"}`,
	} {
		_, ok, err := ParseUserDataEvent([]byte(raw), time.Now())
		if err != nil {
			t.Fatalf("ParseUserDataEvent(%s) error = %v", raw, err)
		}
		if ok {
			t.Fatalf("ParseUserDataEvent(%s) ok = true, want ignored", raw)
		}
	}
}

func TestParseUserDataEventBadJSON(t *testing.T) {
	if _, _, err := ParseUserDataEvent([]byte(`{not json`), time.Now()); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}
