package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"bottrade/internal/config"
	"bottrade/internal/signals"
)

func TestTradingViewWebhookAcceptsSignal(t *testing.T) {
	cfg := testConfig()
	processor := signals.NewProcessor(signals.ProcessorConfig{
		AdminUserID: 12345,
		Logger:      testLogger(),
	})
	server := NewServer(cfg, processor, testLogger())

	req := httptest.NewRequest(http.MethodPost, "/tradingview/webhook", bytes.NewBufferString(`{"secret":"secret","symbol":"BTCUSDT","price":"67500","indicators":{"rsi":28.4}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.App().Test(req)
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var result signals.ProcessResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !result.Accepted {
		t.Fatal("Accepted = false, want true")
	}
	if result.Signal.Symbol != "BTCUSDT" {
		t.Fatalf("Symbol = %q, want BTCUSDT", result.Signal.Symbol)
	}
}

func TestTradingViewWebhookAcceptsTradingViewAliases(t *testing.T) {
	cfg := testConfig()
	processor := signals.NewProcessor(signals.ProcessorConfig{
		AdminUserID: 12345,
		Logger:      testLogger(),
	})
	server := NewServer(cfg, processor, testLogger())

	req := httptest.NewRequest(http.MethodPost, "/tradingview/webhook", bytes.NewBufferString(`{"secret":"secret","ticker":"ETHUSDT","timeframe":"1h","close":"3300"}`))
	resp, err := server.App().Test(req)
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	defer resp.Body.Close()

	var result signals.ProcessResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Signal.Symbol != "ETHUSDT" {
		t.Fatalf("Symbol = %q, want ETHUSDT", result.Signal.Symbol)
	}
	if result.Signal.Price != "3300" {
		t.Fatalf("Price = %q, want 3300", result.Signal.Price)
	}
}

func TestTradingViewWebhookRejectsBadSecret(t *testing.T) {
	cfg := testConfig()
	server := NewServer(cfg, signals.NewProcessor(signals.ProcessorConfig{Logger: testLogger()}), testLogger())

	req := httptest.NewRequest(http.MethodPost, "/tradingview/webhook", bytes.NewBufferString(`{"secret":"wrong","symbol":"BTCUSDT","price":"67500"}`))
	resp, err := server.App().Test(req)
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	server := NewServer(testConfig(), nil, testLogger())

	resp, err := server.App().Test(httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func testConfig() config.Config {
	cfg, err := config.LoadFromLookup(func(key string) (string, bool) {
		values := map[string]string{
			"TELEGRAM_BOT_TOKEN":         "123:abc",
			"TELEGRAM_ALLOWED_USER_IDS":  "12345",
			"MONGODB_URI":                "mongodb+srv://mongo.example.invalid/tradebot",
			"MONGODB_DATABASE":           "tradebot",
			"HTTP_ENABLED":               "true",
			"TRADINGVIEW_ENABLED":        "true",
			"TRADINGVIEW_WEBHOOK_SECRET": "secret",
		}
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		panic(err)
	}
	return cfg
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
