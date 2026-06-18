package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"bottrade/internal/signals"
)

type fakeProvider struct {
	name     string
	category ContextCategory
	fragment ContextFragment
	err      error
	delay    time.Duration
}

func (p fakeProvider) Name() string              { return p.name }
func (p fakeProvider) Category() ContextCategory { return p.category }

func (p fakeProvider) Enrich(ctx context.Context, _ signals.MarketSignal) (ContextFragment, error) {
	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return ContextFragment{}, ctx.Err()
		}
	}
	if p.err != nil {
		return ContextFragment{}, p.err
	}
	return p.fragment, nil
}

func TestAggregatorGatherSkipsFailedAndSlowProviders(t *testing.T) {
	providers := []ContextProvider{
		fakeProvider{
			name:     "grok",
			category: CategoryNarrative,
			fragment: ContextFragment{Summary: "X chatter bullish", Sentiment: "bullish"},
		},
		fakeProvider{
			name:     "nansen",
			category: CategoryOnChain,
			err:      errors.New("rate limited"),
		},
		fakeProvider{
			name:     "cryptoquant",
			category: CategoryOrderFlow,
			delay:    50 * time.Millisecond,
			fragment: ContextFragment{Summary: "should time out"},
		},
		fakeProvider{
			name:     "coinglass",
			category: CategoryOrderFlow,
			fragment: ContextFragment{Summary: "OI +15%", Metrics: map[string]string{"open_interest_change": "+15%"}},
		},
	}

	agg := NewAggregator(AggregatorConfig{
		Providers:       providers,
		ProviderTimeout: 10 * time.Millisecond,
	})

	got := agg.Gather(context.Background(), signals.MarketSignal{Symbol: "HYPEUSDT"})

	if len(got.Fragments) != 2 {
		t.Fatalf("fragments = %d, want 2 (failed + slow skipped)", len(got.Fragments))
	}
	// Provider order is preserved: grok before coinglass.
	if got.Fragments[0].Provider != "grok" || got.Fragments[1].Provider != "coinglass" {
		t.Fatalf("providers = %q, %q; want grok, coinglass", got.Fragments[0].Provider, got.Fragments[1].Provider)
	}
	// Aggregator backfills provider/category from the source when omitted.
	if got.Fragments[0].Category != CategoryNarrative {
		t.Fatalf("category = %q, want narrative", got.Fragments[0].Category)
	}
}

func TestAggregatorGatherNoProviders(t *testing.T) {
	agg := NewAggregator(AggregatorConfig{})
	got := agg.Gather(context.Background(), signals.MarketSignal{Symbol: "BTCUSDT"})
	if !got.IsEmpty() {
		t.Fatalf("IsEmpty = false, want true for no providers")
	}
}

func TestMarketContextPromptIsDeterministicAndOrdered(t *testing.T) {
	ctxData := MarketContext{Fragments: []ContextFragment{
		{Provider: "coinglass", Category: CategoryOrderFlow, Summary: "OI rising", Metrics: map[string]string{"open_interest_change": "+15%", "funding": "neutral"}},
		{Provider: "grok", Category: CategoryNarrative, Sentiment: "bullish", Summary: "narrative hot"},
		{Provider: "nansen", Category: CategoryOnChain, Summary: "whales accumulating"},
	}}

	want := strings.Join([]string{
		"Multi-source market context (weigh these; never invent missing data):",
		"",
		"[narrative]",
		"- grok (bullish): narrative hot",
		"",
		"[onchain]",
		"- nansen: whales accumulating",
		"",
		"[orderflow]",
		"- coinglass: OI rising",
		"    funding=neutral",
		"    open_interest_change=+15%",
	}, "\n")

	if got := ctxData.Prompt(); got != want {
		t.Fatalf("Prompt mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestMarketContextPromptEmpty(t *testing.T) {
	if got := (MarketContext{}).Prompt(); got != "" {
		t.Fatalf("Prompt = %q, want empty", got)
	}
}
