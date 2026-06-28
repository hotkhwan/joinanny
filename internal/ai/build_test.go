package ai

import (
	"testing"
	"time"

	"bottrade/internal/signals"
)

func TestBuildAdvisor(t *testing.T) {
	if _, err := BuildAdvisor(ProviderSpec{Provider: "anthropic", APIKey: "k", Model: "claude-opus-4-8"}, time.Second, nil); err != nil {
		t.Fatalf("anthropic: %v", err)
	}
	if _, err := BuildAdvisor(ProviderSpec{Provider: "openai_compatible", APIKey: "k", Model: "deepseek-chat"}, time.Second, nil); err != nil {
		t.Fatalf("openai_compatible: %v", err)
	}
	if _, err := BuildAdvisor(ProviderSpec{Provider: "nope"}, time.Second, nil); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestBuildEnsemble(t *testing.T) {
	if _, err := BuildEnsemble(nil, "", 0, time.Second, nil); err == nil {
		t.Fatal("expected error for no providers")
	}

	// One provider -> the advisor itself (not an ensemble wrapper).
	single, err := BuildEnsemble([]ProviderSpec{
		{Name: "claude", Provider: "anthropic", APIKey: "k", Model: "claude-opus-4-8"},
	}, "", 0, time.Second, nil)
	if err != nil {
		t.Fatalf("single: %v", err)
	}
	if _, isEnsemble := single.(*EnsembleAdvisor); isEnsemble {
		t.Fatal("single provider should not be wrapped in an ensemble")
	}

	// Several providers -> an ensemble.
	panel, err := BuildEnsemble([]ProviderSpec{
		{Name: "claude", Provider: "anthropic", APIKey: "k", Model: "claude-opus-4-8"},
		{Name: "deepseek", Provider: "openai_compatible", APIKey: "k", Model: "deepseek-chat", BaseURL: "https://api.deepseek.com/v1"},
		{Name: "qwen", Provider: "openai_compatible", APIKey: "k", Model: "qwen-max", BaseURL: "https://example/v1"},
	}, "consensus", 0, time.Second, nil)
	if err != nil {
		t.Fatalf("panel: %v", err)
	}
	if _, isEnsemble := panel.(*EnsembleAdvisor); !isEnsemble {
		t.Fatalf("multiple providers should build an ensemble, got %T", panel)
	}
	var _ signals.Advisor = panel
}
