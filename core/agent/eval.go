package agent

import (
	"context"
	"fmt"

	"gogogot/core/event"
	"gogogot/infra/llm/types"
)

type EvalResult struct {
	Passed   bool
	Score    float64
	Feedback string
	Details  map[string]any
}

type Evaluator interface {
	Evaluate(ctx context.Context) EvalResult
}

func (a *Agent) RunWithEval(ctx context.Context, task string, eval Evaluator) error {
	maxIters := a.config.EvalIterations
	if maxIters <= 0 {
		maxIters = 1
	}

	for iter := 0; iter < maxIters; iter++ {
		err := a.Run(ctx, []types.ContentBlock{types.TextBlock(task)})
		if err != nil {
			return err
		}

		if eval == nil {
			break
		}

		a.emit(event.EvalRun, map[string]any{"iteration": iter})
		result := eval.Evaluate(ctx)
		a.emit(event.EvalResult, map[string]any{
			"iteration": iter,
			"passed":    result.Passed,
			"feedback":  result.Feedback,
		})

		if result.Passed {
			return nil
		}

		a.session.CompactAll("attempt failed: " + result.Feedback)
		task = fmt.Sprintf("Previous attempt failed.\nFeedback: %s\nOriginal task: %s", result.Feedback, task)
	}

	if eval != nil {
		return fmt.Errorf("eval loop: max iterations (%d) reached", maxIters)
	}
	return nil
}
