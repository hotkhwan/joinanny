package ai

import (
	"errors"
	"fmt"
	"time"

	"bottrade/internal/signals"
)

// ProviderSpec describes one AI advisor to build (one panel member).
type ProviderSpec struct {
	Name     string // label recorded in the journal; defaults to Provider
	Provider string // "anthropic" | "openai_compatible"
	APIKey   string
	BaseURL  string
	Model    string
}

// BuildAdvisor constructs a single advisor from a spec. DeepSeek and Qwen use
// "openai_compatible" with their own base URLs; Claude uses "anthropic".
func BuildAdvisor(spec ProviderSpec, timeout time.Duration, enricher ContextEnricher) (signals.Advisor, error) {
	switch spec.Provider {
	case "anthropic":
		return NewAnthropicAdvisor(AnthropicConfig{
			APIKey:         spec.APIKey,
			BaseURL:        spec.BaseURL,
			Model:          spec.Model,
			RequestTimeout: timeout,
			Enricher:       enricher,
		}), nil
	case "openai_compatible":
		return NewOpenAICompatibleAdvisor(OpenAICompatibleConfig{
			APIKey:         spec.APIKey,
			BaseURL:        spec.BaseURL,
			Model:          spec.Model,
			RequestTimeout: timeout,
			Enricher:       enricher,
		}), nil
	default:
		return nil, fmt.Errorf("ai: unsupported provider %q", spec.Provider)
	}
}

// BuildEnsemble returns a single advisor when one spec is given, or an
// EnsembleAdvisor (panel vote) when several are. policy and minVotes apply only
// to the multi-advisor case.
func BuildEnsemble(specs []ProviderSpec, policy string, minVotes int, timeout time.Duration, enricher ContextEnricher) (signals.Advisor, error) {
	if len(specs) == 0 {
		return nil, errors.New("ai: no providers configured")
	}

	named := make([]NamedAdvisor, 0, len(specs))
	for _, spec := range specs {
		advisor, err := BuildAdvisor(spec, timeout, enricher)
		if err != nil {
			return nil, err
		}
		name := spec.Name
		if name == "" {
			name = spec.Provider
		}
		named = append(named, NamedAdvisor{Name: name, Advisor: advisor})
	}

	if len(named) == 1 {
		return named[0].Advisor, nil
	}
	return NewEnsembleAdvisor(EnsembleConfig{
		Advisors: named,
		Policy:   AggregationPolicy(policy),
		MinVotes: minVotes,
	})
}
