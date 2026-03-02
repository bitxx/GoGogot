package agent

import (
	"context"
	"fmt"

	"gogogot/core/agent/orchestration"
)

func (a *Agent) RunWithEval(ctx context.Context, task string, eval orchestration.Evaluator) error {
	maxIters := a.config.EvalIterations
	if maxIters <= 0 {
		maxIters = 1
	}

	for iter := 0; iter < maxIters; iter++ {
		err := a.Run(ctx, task)
		if err != nil {
			return err
		}

		if eval == nil {
			break
		}

		a.emit(orchestration.EventEvalRun, map[string]any{"iteration": iter})
		result := eval.Evaluate(ctx)
		a.emit(orchestration.EventEvalResult, map[string]any{
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
