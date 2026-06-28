package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bottrade/internal/signals"
)

func TestOpenAICompatibleAdvisorParsesDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want bearer key", got)
		}

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.ResponseFormat["type"] != "json_object" {
			t.Fatalf("response_format = %#v, want json_object", req.ResponseFormat)
		}

		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"action\":\"hold\",\"symbol\":\"BTCUSDT\",\"confidence_percent\":72,\"reason\":\"No clean setup\"}"}}]}`))
	}))
	defer server.Close()

	advisor := NewOpenAICompatibleAdvisor(OpenAICompatibleConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		Model:          "test-model",
		RequestTimeout: time.Second,
	})

	decision, err := advisor.Decide(context.Background(), signals.MarketSignal{
		Symbol: "BTCUSDT",
		Price:  "67500",
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.Action != signals.ActionHold {
		t.Fatalf("Action = %q, want hold", decision.Action)
	}
	if decision.ConfidencePercent != 72 {
		t.Fatalf("Confidence = %d, want 72", decision.ConfidencePercent)
	}
}

type staticEnricher struct {
	context MarketContext
}

func (e staticEnricher) Gather(context.Context, signals.MarketSignal) MarketContext {
	return e.context
}

func TestOpenAICompatibleAdvisorInjectsEnrichedContext(t *testing.T) {
	var capturedUser string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				capturedUser = msg.Content
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"action\":\"hold\",\"symbol\":\"HYPEUSDT\",\"confidence_percent\":40,\"reason\":\"mixed\"}"}}]}`))
	}))
	defer server.Close()

	advisor := NewOpenAICompatibleAdvisor(OpenAICompatibleConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		Model:          "test-model",
		RequestTimeout: time.Second,
		Enricher: staticEnricher{context: MarketContext{Fragments: []ContextFragment{
			{Provider: "nansen", Category: CategoryOnChain, Summary: "whales accumulating"},
		}}},
	})

	if _, err := advisor.Decide(context.Background(), signals.MarketSignal{Symbol: "HYPEUSDT", Price: "30"}); err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}

	if !strings.Contains(capturedUser, "whales accumulating") {
		t.Fatalf("user message missing enriched context, got:\n%s", capturedUser)
	}
	if !strings.Contains(capturedUser, "Analyze this TradingView signal") {
		t.Fatalf("user message missing base signal prompt, got:\n%s", capturedUser)
	}
}
