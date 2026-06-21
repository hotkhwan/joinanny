package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestReadyzExposesNoTradingConfig(t *testing.T) {
	server := NewServer(testConfig(), nil, testLogger())

	resp, err := server.App().Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	for _, leak := range []string{"dry_run", "autotrade", "tradingview", "\"ai\""} {
		if bytes.Contains(body, []byte(leak)) {
			t.Fatalf("/readyz leaked %q: %s", leak, body)
		}
	}
}

func TestStatusDisabledWithoutToken(t *testing.T) {
	server := NewServer(testConfig(), nil, testLogger())

	resp, err := server.App().Test(httptest.NewRequest(http.MethodGet, "/status", nil))
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when no token configured", resp.StatusCode)
	}
}

func TestStatusRequiresBearerToken(t *testing.T) {
	cfg := testConfigWith(t, map[string]string{"HTTP_STATUS_TOKEN": "s3cr3t-token"})
	server := NewServer(cfg, nil, testLogger())

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"missing prefix", "s3cr3t-token", http.StatusUnauthorized},
		{"correct token", "Bearer s3cr3t-token", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/status", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp, err := server.App().Test(req)
			if err != nil {
				t.Fatalf("Test returned error: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want == http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				if !bytes.Contains(body, []byte("dry_run")) {
					t.Fatalf("authorized /status missing flags: %s", body)
				}
			}
		})
	}
}

func TestWebhookRateLimited(t *testing.T) {
	server := NewServer(testConfig(), signals.NewProcessor(signals.ProcessorConfig{Logger: testLogger()}), testLogger())

	var last int
	for i := 0; i < 31; i++ {
		req := httptest.NewRequest(http.MethodPost, "/tradingview/webhook", bytes.NewBufferString(`{"secret":"wrong"}`))
		resp, err := server.App().Test(req)
		if err != nil {
			t.Fatalf("request %d error: %v", i, err)
		}
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("request 31 status = %d, want 429 (rate limit not enforced)", last)
	}
}

func TestWebhookBodyLimit(t *testing.T) {
	server := NewServer(testConfig(), signals.NewProcessor(signals.ProcessorConfig{Logger: testLogger()}), testLogger())

	big := bytes.Repeat([]byte("a"), webhookBodyLimit+1)
	resp, err := server.App().Test(httptest.NewRequest(http.MethodPost, "/tradingview/webhook", bytes.NewReader(big)))
	if err != nil {
		// Fiber rejects oversized bodies at the transport layer; app.Test
		// surfaces that as an error instead of a 413 response. Either way the
		// request is rejected, which is the behavior under test.
		if !strings.Contains(err.Error(), "limit") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 for oversized body", resp.StatusCode)
	}
}

func testConfigWith(t *testing.T, overrides map[string]string) config.Config {
	t.Helper()
	values := map[string]string{
		"TELEGRAM_BOT_TOKEN":         "123:abc",
		"TELEGRAM_ALLOWED_USER_IDS":  "12345",
		"MONGODB_URI":                "mongodb+srv://mongo.example.invalid/tradebot",
		"MONGODB_DATABASE":           "tradebot",
		"HTTP_ENABLED":               "true",
		"TRADINGVIEW_ENABLED":        "true",
		"TRADINGVIEW_WEBHOOK_SECRET": "secret",
	}
	for k, v := range overrides {
		values[k] = v
	}
	cfg, err := config.LoadFromLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("config load: %v", err)
	}
	return cfg
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
