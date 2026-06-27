// Package realtime turns Binance futures position activity into a stream of
// events that fan out to the web dashboard (SSE) and Telegram. The live half is
// a Gateway that snapshots open positions on an interval and emits an event
// whenever a position opens, changes, or closes — so a stop-loss or take-profit
// fill (which never passes through the order service) is still surfaced and
// journaled. A Binance user-data WebSocket is a drop-in upgrade behind the same
// event model: ParseUserDataEvent already decodes ORDER_TRADE_UPDATE /
// ACCOUNT_UPDATE into the same Event, so the WS path can replace polling without
// touching subscribers. See PRODUCTIONIZATION.md.
package realtime

import (
	"encoding/json"
	"strings"
	"time"

	"bottrade/internal/decimal"
)

// EventType classifies a realtime event.
type EventType string

const (
	// EventPositionUpdate is an open position that is new or whose size / mark
	// price changed since the last snapshot.
	EventPositionUpdate EventType = "position_update"
	// EventTradeClosed is a position that went flat — closed by a manual close,
	// a stop-loss, or a take-profit. RealizedPnL carries the round-trip result.
	EventTradeClosed EventType = "trade_closed"
)

// Event is a single realtime update for one symbol. It is JSON-encodable so the
// SSE handler can write it straight to the wire.
type Event struct {
	Type          EventType       `json:"type"`
	UserID        int64           `json:"user_id"`
	Symbol        string          `json:"symbol"`
	Side          string          `json:"side"`
	Amount        decimal.Decimal `json:"amount"`
	EntryPrice    decimal.Decimal `json:"entry_price"`
	MarkPrice     decimal.Decimal `json:"mark_price"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	RealizedPnL   decimal.Decimal `json:"realized_pnl"`
	At            time.Time       `json:"at"`
}

// userDataEnvelope is the outer shape of a Binance futures user-data message.
// Only the fields the gateway surfaces are decoded.
type userDataEnvelope struct {
	EventType string `json:"e"`
	EventTime int64  `json:"E"`
	Order     *struct {
		Symbol        string `json:"s"`
		Side          string `json:"S"`
		ExecutionType string `json:"x"`
		OrderStatus   string `json:"X"`
		RealizedPnL   string `json:"rp"`
		AveragePrice  string `json:"ap"`
		ReduceOnly    bool   `json:"R"`
	} `json:"o"`
}

// ParseUserDataEvent decodes a raw Binance user-data WebSocket message into an
// Event. ok is false for messages this package does not surface (keepalives,
// account config changes, fills that neither open nor close a tracked position).
// It is the seam for a future live WebSocket stream; the polling Gateway does not
// use it. now is the fallback timestamp when the message omits an event time.
func ParseUserDataEvent(raw []byte, now time.Time) (Event, bool, error) {
	var env userDataEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Event{}, false, err
	}
	if env.EventType != "ORDER_TRADE_UPDATE" || env.Order == nil {
		return Event{}, false, nil
	}
	order := env.Order
	// Only completed fills carry a settled position change.
	if !strings.EqualFold(order.ExecutionType, "TRADE") || !strings.EqualFold(order.OrderStatus, "FILLED") {
		return Event{}, false, nil
	}

	at := now
	if env.EventTime > 0 {
		at = time.UnixMilli(env.EventTime).UTC()
	}

	realized := parseDecimalOrZero(order.RealizedPnL)
	// A reduce-only fill, or any fill that settled a realized PnL, closed (part of)
	// a position. An opening fill is reduceOnly=false with zero realized PnL.
	if order.ReduceOnly || !realized.IsZero() {
		return Event{
			Type:        EventTradeClosed,
			Symbol:      order.Symbol,
			Side:        strings.ToLower(order.Side),
			MarkPrice:   parseDecimalOrZero(order.AveragePrice),
			RealizedPnL: realized,
			At:          at,
		}, true, nil
	}

	return Event{
		Type:      EventPositionUpdate,
		Symbol:    order.Symbol,
		Side:      strings.ToLower(order.Side),
		MarkPrice: parseDecimalOrZero(order.AveragePrice),
		At:        at,
	}, true, nil
}

func parseDecimalOrZero(value string) decimal.Decimal {
	value = strings.TrimSpace(value)
	if value == "" {
		return decimal.Zero()
	}
	parsed, err := decimal.Parse(value)
	if err != nil {
		return decimal.Zero()
	}
	return parsed
}
