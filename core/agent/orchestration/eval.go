package orchestration

import "context"

type EvalResult struct {
	Passed   bool
	Score    float64
	Feedback string
	Details  map[string]any
}

type Evaluator interface {
	Evaluate(ctx context.Context) EvalResult
}
