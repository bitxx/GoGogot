package llm

import "gogogot/infra/config"

type Provider struct {
	ID            string
	Label         string
	Model         string
	BaseURL       string
	APIKey        string
	Format        string // "anthropic" (default) or "openai"
	ContextWindow int
}

func AvailableProviders(cfg *config.Config) []Provider {
	var providers []Provider

	if cfg.AnthropicKey != "" {
		providers = append(providers, Provider{
			ID:            "claude",
			Label:         "Claude Sonnet 4.6 (Anthropic)",
			Model:         "claude-sonnet-4-6",
			APIKey:        cfg.AnthropicKey,
			ContextWindow: 200_000,
		})
	}

	if cfg.OpenRouterKey != "" {
		providers = append(providers, Provider{
			ID:            "minimax",
			Label:         "MiniMax M2.5 (OpenRouter)",
			Model:         "minimax/minimax-m2.5",
			BaseURL:       "https://openrouter.ai/api/v1",
			APIKey:        cfg.OpenRouterKey,
			Format:        "openai",
			ContextWindow: 1_000_000,
		})
	}

	return providers
}
