package signals

import (
	"context"
	"testing"
	"time"
)

func TestDecisionToIntentOpen(t *testing.T) {
	decision := Decision{
		Action:            ActionOpen,
		Symbol:            "BTC",
		Side:              "long",
		Leverage:          3,
		Entry:             "67500",
		StopLoss:          "65000",
		TakeProfit:        "72000",
		SizeUSDT:          "100",
		ConfidencePercent: 80,
	}

	intent, err := DecisionToIntent(decision, 20)
	if err != nil {
		t.Fatalf("DecisionToIntent returned error: %v", err)
	}

	if intent.Open.Symbol != "BTCUSDT" {
		t.Fatalf("Symbol = %q, want BTCUSDT", intent.Open.Symbol)
	}
}

func TestMarketSignalValidateRequiresSymbol(t *testing.T) {
	err := MarketSignal{Price: "100"}.Validate()
	if err == nil {
		t.Fatal("Validate returned nil error, want symbol error")
	}
}

func TestProcessorRecordsSignal(t *testing.T) {
	store := &recordingSignalStore{}
	processor := NewProcessor(ProcessorConfig{
		SignalStore: store,
	})

	result, err := processor.Process(context.Background(), MarketSignal{
		Source:     "tradingview",
		Symbol:     "BTC",
		Price:      "67500",
		ReceivedAt: time.Unix(1710000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("Accepted = false, want true")
	}
	if len(store.records) != 1 {
		t.Fatalf("records = %d, want 1", len(store.records))
	}
	if store.records[0].Status != SignalStatusHeld {
		t.Fatalf("status = %q, want held", store.records[0].Status)
	}
	if store.records[0].Signal.Symbol != "BTCUSDT" {
		t.Fatalf("symbol = %q, want BTCUSDT", store.records[0].Signal.Symbol)
	}
}

type recordingSignalStore struct {
	records []SignalRecord
}

func (s *recordingSignalStore) PutSignalRecord(ctx context.Context, record SignalRecord) error {
	s.records = append(s.records, record)
	return nil
}
