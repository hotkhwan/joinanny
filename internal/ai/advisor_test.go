package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
