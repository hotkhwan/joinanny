package parser

import (
	"strings"
	"testing"

	"bottrade/internal/domain"
)

func TestParseOpenLongSizeUSDT(t *testing.T) {
	intent, err := Parse("long BTC 3x entry 67500 sl 65000 tp 72000 size 100usdt", Options{MaxLeverage: 20})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if intent.Type != domain.IntentOpen {
		t.Fatalf("Type = %q, want open", intent.Type)
	}
	open := intent.Open
	if open == nil {
		t.Fatal("Open is nil")
	}
	if open.Symbol != "BTCUSDT" {
		t.Fatalf("Symbol = %q, want BTCUSDT", open.Symbol)
	}
	if open.Side != domain.SideLong {
		t.Fatalf("Side = %q, want long", open.Side)
	}
	if open.Leverage != 3 {
		t.Fatalf("Leverage = %d, want 3", open.Leverage)
	}
	if got := open.Entry.String(); got != "67500" {
		t.Fatalf("Entry = %q, want 67500", got)
	}
	if got := open.TakeProfits[0].String(); got != "72000" {
		t.Fatalf("TP = %q, want 72000", got)
	}
	if open.Size.Kind != domain.SizeUSDT {
		t.Fatalf("Size kind = %q, want usdt", open.Size.Kind)
	}
	if got := open.Size.Amount.String(); got != "100" {
		t.Fatalf("Size amount = %q, want 100", got)
	}
}

func TestParseOpenShortQtyWithPlan(t *testing.T) {
	intent, err := Parse("short ETH 2x entry 3300 sl 3450 tp 3000 qty 0.05 plan 2", Options{MaxLeverage: 20})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	open := intent.Open
	if open.Symbol != "ETHUSDT" {
		t.Fatalf("Symbol = %q, want ETHUSDT", open.Symbol)
	}
	if open.Side != domain.SideShort {
		t.Fatalf("Side = %q, want short", open.Side)
	}
	if open.Size.Kind != domain.SizeQty {
		t.Fatalf("Size kind = %q, want qty", open.Size.Kind)
	}
	if got := open.Size.Amount.String(); got != "0.05" {
		t.Fatalf("Size amount = %q, want 0.05", got)
	}
	if open.PlanID != "2" {
		t.Fatalf("PlanID = %q, want 2", open.PlanID)
	}
}

func TestParseCloseCommands(t *testing.T) {
	tests := []struct {
		text       string
		all        bool
		symbol     string
		percent    string
		hasPercent bool
	}{
		{text: "close all", all: true, percent: "100"},
		{text: "close BTC", symbol: "BTCUSDT", percent: "100"},
		{text: "close ETH 50%", symbol: "ETHUSDT", percent: "50", hasPercent: true},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			intent, err := Parse(tt.text, Options{})
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}

			if intent.Type != domain.IntentClose {
				t.Fatalf("Type = %q, want close", intent.Type)
			}
			closeIntent := intent.Close
			if closeIntent.All != tt.all {
				t.Fatalf("All = %v, want %v", closeIntent.All, tt.all)
			}
			if closeIntent.Symbol != tt.symbol {
				t.Fatalf("Symbol = %q, want %q", closeIntent.Symbol, tt.symbol)
			}
			if closeIntent.HasPercent != tt.hasPercent {
				t.Fatalf("HasPercent = %v, want %v", closeIntent.HasPercent, tt.hasPercent)
			}
			if got := closeIntent.ResolvedPercent.String(); got != tt.percent {
				t.Fatalf("ResolvedPercent = %q, want %q", got, tt.percent)
			}
		})
	}
}

func TestParseReadOnlyCommands(t *testing.T) {
	tests := []struct {
		text string
		want domain.IntentType
	}{
		{text: "status", want: domain.IntentStatus},
		{text: "/status", want: domain.IntentStatus},
		{text: "plan 1 status", want: domain.IntentPlanStatus},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			intent, err := Parse(tt.text, Options{})
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if intent.Type != tt.want {
				t.Fatalf("Type = %q, want %q", intent.Type, tt.want)
			}
		})
	}
}

func TestParsePhaseTwoManagementCommands(t *testing.T) {
	tests := []struct {
		text string
		want domain.IntentType
	}{
		{text: "be BTC", want: domain.IntentBreakeven},
		{text: "move sl BTC to be", want: domain.IntentBreakeven},
		{text: "trail BTC 0.5%", want: domain.IntentTrail},
		{text: "add BTC size 100usdt", want: domain.IntentAdd},
		{text: "add ETH qty 0.05", want: domain.IntentAdd},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			intent, err := Parse(tt.text, Options{})
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if intent.Type != tt.want {
				t.Fatalf("Type = %q, want %q", intent.Type, tt.want)
			}
			if !intent.IsExchangeChanging() {
				t.Fatalf("intent %q should require confirmation", intent.Type)
			}
		})
	}
}

func TestParseRejectsInvalidOpenOrders(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "missing size",
			text: "long BTC 3x entry 67500 sl 65000 tp 72000",
			want: "Open order format",
		},
		{
			name: "long stop above entry",
			text: "long BTC 3x entry 67500 sl 68000 tp 72000 size 100usdt",
			want: "Long stop loss must be below entry",
		},
		{
			name: "short take profit above entry",
			text: "short ETH 2x entry 3300 sl 3450 tp 3600 qty 0.05",
			want: "Short take profit must be below entry",
		},
		{
			name: "leverage too high",
			text: "long BTC 21x entry 67500 sl 65000 tp 72000 size 100usdt",
			want: "exceeds MAX_LEVERAGE",
		},
		{
			name: "bad size unit",
			text: "long BTC 3x entry 67500 sl 65000 tp 72000 size 100",
			want: "size must use",
		},
		{
			name: "bad plan",
			text: "long BTC 3x entry 67500 sl 65000 tp 72000 size 100usdt plan 9",
			want: "Plan must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.text, Options{MaxLeverage: 20})
			if err == nil {
				t.Fatal("Parse returned nil error, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestParseRejectsInvalidClosePercent(t *testing.T) {
	_, err := Parse("close BTC 101%", Options{})
	if err == nil {
		t.Fatal("Parse returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "at most 100") {
		t.Fatalf("error = %q, want percentage validation", err.Error())
	}
}

func TestParseRejectsInvalidTrailPercent(t *testing.T) {
	_, err := Parse("trail BTC 11%", Options{})
	if err == nil {
		t.Fatal("Parse returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "at most 10") {
		t.Fatalf("error = %q, want callback validation", err.Error())
	}
}
