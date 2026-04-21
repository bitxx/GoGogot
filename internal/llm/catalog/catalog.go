package catalog

import (
	_ "embed"
	"encoding/json"
)

type ModelDef struct {
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	ContextWindow   int     `json:"context_window"`
	Vision          bool    `json:"vision"`
	InputPricePerM  float64 `json:"input_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`
}

//go:embed openai.json
var openaiJSON []byte

//go:embed anthropic.json
var anthropicJSON []byte

//go:embed deepseek.json
var deepseekJSON []byte

func OpenAI() map[string]ModelDef    { return loadClean(openaiJSON) }
func Anthropic() map[string]ModelDef { return loadClean(anthropicJSON) }
func DeepSeek() map[string]ModelDef  { return loadClean(deepseekJSON) }

func loadClean(data []byte) map[string]ModelDef {
	var models []ModelDef
	if err := json.Unmarshal(data, &models); err != nil {
		return nil
	}
	m := make(map[string]ModelDef, len(models))
	for _, def := range models {
		m[def.ID] = def
	}
	return m
}
