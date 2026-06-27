package config

import "testing"

func TestLoadAIProviders(t *testing.T) {
	cfg, err := LoadFromLookup(testLookup(map[string]string{
		"AI_PROVIDERS":          `[{"name":"claude","provider":"anthropic","api_key":"k1","model":"claude-opus-4-8"},{"name":"deepseek","provider":"openai_compatible","api_key":"k2","base_url":"https://api.deepseek.com/v1","model":"deepseek-chat"}]`,
		"AI_ENSEMBLE_POLICY":    "consensus",
		"AI_ENSEMBLE_MIN_VOTES": "2",
	}))
	if err != nil {
		t.Fatalf("LoadFromLookup returned error: %v", err)
	}
	if len(cfg.AI.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(cfg.AI.Providers))
	}
	if cfg.AI.Providers[0].Name != "claude" || cfg.AI.Providers[0].Provider != "anthropic" {
		t.Fatalf("provider[0] = %+v", cfg.AI.Providers[0])
	}
	if cfg.AI.Providers[1].Model != "deepseek-chat" || cfg.AI.Providers[1].BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("provider[1] = %+v", cfg.AI.Providers[1])
	}
	if cfg.AI.EnsemblePolicy != "consensus" || cfg.AI.EnsembleMinVotes != 2 {
		t.Fatalf("policy = %q minVotes = %d", cfg.AI.EnsemblePolicy, cfg.AI.EnsembleMinVotes)
	}
	if !cfg.AI.Enabled {
		t.Fatal("AI should be enabled when providers are configured")
	}
}

func TestLoadAIProvidersRejectsBadJSON(t *testing.T) {
	if _, err := LoadFromLookup(testLookup(map[string]string{"AI_PROVIDERS": "not json"})); err == nil {
		t.Fatal("expected validation error for malformed AI_PROVIDERS")
	}
}
