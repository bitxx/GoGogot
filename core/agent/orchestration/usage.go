package orchestration

import (
	"strings"
	"time"
)

type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalTokens      int
	LLMCalls         int
	ToolCalls        int
	Cost             float64 // estimated USD
	Duration         time.Duration
}

func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheReadTokens += other.CacheReadTokens
	u.CacheWriteTokens += other.CacheWriteTokens
	u.TotalTokens += other.TotalTokens
	u.LLMCalls += other.LLMCalls
	u.ToolCalls += other.ToolCalls
	u.Cost += other.Cost
	u.Duration += other.Duration
}

// per-million-token pricing
type modelPricing struct {
	InputPerM  float64
	OutputPerM float64
}

var knownPricing = map[string]modelPricing{
	"claude-sonnet-4-6":   {InputPerM: 3.0, OutputPerM: 15.0},
	"minimax/minimax-m2.5": {InputPerM: 0.5, OutputPerM: 1.5},
}

// CalcCost estimates USD cost for a single LLM call.
// Falls back to zero if model is unknown.
func CalcCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := knownPricing[model]
	if !ok {
		for k, v := range knownPricing {
			if strings.Contains(model, k) || strings.Contains(k, model) {
				p = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0
	}
	return float64(inputTokens)/1_000_000*p.InputPerM +
		float64(outputTokens)/1_000_000*p.OutputPerM
}
