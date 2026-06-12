package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bottrade/internal/signals"
)

const defaultSystemPrompt = `You are a trading decision engine for a Binance Futures testnet bot.
Return only JSON. Prefer hold when the signal is weak or incomplete.
Never invent missing prices. Any open decision must include symbol, side, leverage, entry, stop_loss, take_profit, and either size_usdt or qty.
Use conservative leverage and explain the technical reason briefly.`

type OpenAICompatibleConfig struct {
	APIKey         string
	BaseURL        string
	Model          string
	SystemPrompt   string
	RequestTimeout time.Duration
	HTTPClient     *http.Client
}

type OpenAICompatibleAdvisor struct {
	cfg    OpenAICompatibleConfig
	client *http.Client
}

func NewOpenAICompatibleAdvisor(cfg OpenAICompatibleConfig) *OpenAICompatibleAdvisor {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = defaultSystemPrompt
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 20 * time.Second
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.RequestTimeout}
	}

	return &OpenAICompatibleAdvisor{cfg: cfg, client: client}
}

func (a *OpenAICompatibleAdvisor) Decide(ctx context.Context, signal signals.MarketSignal) (signals.Decision, error) {
	if strings.TrimSpace(a.cfg.APIKey) == "" {
		return signals.Decision{}, fmt.Errorf("AI_API_KEY is required")
	}
	if strings.TrimSpace(a.cfg.Model) == "" {
		return signals.Decision{}, fmt.Errorf("AI_MODEL is required")
	}

	payload := chatCompletionRequest{
		Model: a.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: a.cfg.SystemPrompt},
			{Role: "user", Content: signalPrompt(signal)},
		},
		Temperature: 0.1,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return signals.Decision{}, err
	}

	endpoint := strings.TrimRight(a.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return signals.Decision{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return signals.Decision{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return signals.Decision{}, err
	}
	if resp.StatusCode >= 400 {
		return signals.Decision{}, fmt.Errorf("AI API returned %d: %s", resp.StatusCode, string(responseBody))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return signals.Decision{}, fmt.Errorf("decode AI API response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return signals.Decision{}, fmt.Errorf("AI API returned no choices")
	}

	var decision signals.Decision
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &decision); err != nil {
		return signals.Decision{}, fmt.Errorf("decode AI decision JSON: %w", err)
	}

	return decision, nil
}

func signalPrompt(signal signals.MarketSignal) string {
	data, err := json.Marshal(signal)
	if err != nil {
		return "{}"
	}

	return `Analyze this TradingView signal and return a decision JSON with this shape:
{
  "action": "hold|open|close",
  "symbol": "BTCUSDT",
  "side": "long|short",
  "leverage": 1,
  "entry": "decimal string",
  "stop_loss": "decimal string",
  "take_profit": "decimal string",
  "size_usdt": "decimal string",
  "qty": "decimal string",
  "close_percent": "decimal string 0-100",
  "confidence_percent": 0,
  "reason": "brief reason"
}

Signal:
` + string(data)
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}
