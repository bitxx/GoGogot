package llm

import (
	"fmt"
	"gogogot/internal/llm/catalog"
	"os"
	"strings"
	"sync"
)

type Provider struct {
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	Model           string  `json:"model"`
	BaseURL         string  `json:"base_url,omitempty"`
	APIKey          string  `json:"-"`
	Format          string  `json:"format,omitempty"`
	ContextWindow   int     `json:"context_window"`
	SupportsVision  bool    `json:"supports_vision,omitempty"`
	InputPricePerM  float64 `json:"-"`
	OutputPricePerM float64 `json:"-"`
}

var aliases = map[string]string{
	"claude":   "claude-sonnet-4-6",
	"deepseek": "deepseek/deepseek-v3.2",
	"gemini":   "google/gemini-3-flash-preview",
	"minimax":  "minimax/minimax-m2.5",
	"qwen":     "qwen/qwen3.5-397b-a17b",
	"llama":    "meta-llama/llama-4-maverick",
	"kimi":     "moonshotai/kimi-k2.5",
	"openai":   "openai/gpt-5-nano",
}

var (
	anthropicOnce    sync.Once
	anthropicCatalog map[string]catalog.ModelDef

	openaiOnce    sync.Once
	openaiCatalog map[string]catalog.ModelDef

	deepseekOnce    sync.Once
	deepseekCatalog map[string]catalog.ModelDef
)

func getAnthropicCatalog() map[string]catalog.ModelDef {
	anthropicOnce.Do(func() { anthropicCatalog = catalog.Anthropic() })
	return anthropicCatalog
}

func getOpenAICatalog() map[string]catalog.ModelDef {
	openaiOnce.Do(func() { openaiCatalog = catalog.OpenAI() })
	return openaiCatalog
}

func getDeepSeekCatalog() map[string]catalog.ModelDef {
	deepseekOnce.Do(func() { deepseekCatalog = catalog.DeepSeek() })
	return deepseekCatalog
}

// ResolveProvider builds a Provider from an exact model ID and provider name.
func ResolveProvider(modelID, provider string) (*Provider, error) {
	if resolved, ok := aliases[modelID]; ok {
		modelID = resolved
	}

	switch provider {
	case "anthropic":
		if _, ok := getAnthropicCatalog()[modelID]; !ok {
			return nil, fmt.Errorf("unknown Anthropic model %q — available: %s", modelID, catalogKeys(getAnthropicCatalog()))
		}
		return resolveAnthropic(modelID)

	case "openai":
		if _, ok := getOpenAICatalog()[modelID]; !ok {
			return nil, fmt.Errorf("unknown OpenAI model %q — available: %s", modelID, catalogKeys(getOpenAICatalog()))
		}
		return resolveOpenAI(modelID)

	case "deepseek":
		if _, ok := getDeepSeekCatalog()[modelID]; !ok {
			return nil, fmt.Errorf("unknown DeepSeek model %q — available: %s", modelID, catalogKeys(getDeepSeekCatalog()))
		}
		return resolveDeepSeek(modelID)

	default:
		return nil, fmt.Errorf("unknown provider %q — use 'anthropic', 'openai', or 'deepseek'", provider)
	}
}

func resolveAnthropic(model string) (*Provider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set for model %q", model)
	}
	return providerFromDef(getAnthropicCatalog()[model], model, apiKey, "", "anthropic"), nil
}

func resolveOpenAI(model string) (*Provider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set for model %q", model)
	}
	return providerFromDef(getOpenAICatalog()[model], model, apiKey, "https://api.openai.com/v1", "openai"), nil
}

func resolveDeepSeek(model string) (*Provider, error) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY not set for model %q", model)
	}
	return providerFromDef(getDeepSeekCatalog()[model], model, apiKey, "https://api.deepseek.com/anthropic", "anthropic"), nil
}

func providerFromDef(def catalog.ModelDef, model, apiKey, baseURL, format string) *Provider {
	return &Provider{
		ID: model, Label: def.Label, Model: model,
		BaseURL: baseURL, APIKey: apiKey, Format: format,
		ContextWindow: def.ContextWindow, SupportsVision: def.Vision,
		InputPricePerM: def.InputPricePerM, OutputPricePerM: def.OutputPricePerM,
	}
}

func catalogKeys(m map[string]catalog.ModelDef) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
