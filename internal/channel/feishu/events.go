package feishu

import (
	"context"
	"time"

	"gogogot/internal/core/transport"
)

const (
	minEditInterval = 800 * time.Millisecond
	thinkingDelay   = 10 * time.Second
)

func buildToolStatus(d transport.ToolStartData, plan []transport.PlanTask) transport.AgentStatus {
	label := d.Label
	if label == "" {
		label = d.Name
	}
	if d.Detail != "" {
		label = label + ": " + d.Detail
	}
	phase := transport.Phase(d.Phase)
	if phase == "" {
		phase = transport.PhaseTool
	}
	return transport.AgentStatus{Phase: phase, Tool: d.Name, Detail: label, Plan: plan}
}

func formatMessageWithLevel(text string, level transport.MessageLevel) string {
	switch level {
	case transport.LevelSuccess:
		return "✅ " + text
	case transport.LevelWarning:
		return "⚠️ " + text
	default:
		return "💡 " + text
	}
}

// ConsumeEvents is the exact mirror of the Telegram implementation.
// The only structural difference: statusID is a string (Feishu message_id)
// instead of an int, and rate-limit comparisons use the card JSON string
// instead of formatted Markdown text.
func (r *replier) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	_ = r.SendTyping(ctx)
	statusID := r.sendStatus(ctx, transport.AgentStatus{Phase: transport.PhaseThinking})

	var (
		finalText   string
		currentPlan []transport.PlanTask

		pending      *transport.AgentStatus
		lastJSON     string
		lastEditTime time.Time

		flushTimer *time.Timer
		flushCh    <-chan time.Time

		hadTool    bool
		thinkTimer *time.Timer
		thinkCh    <-chan time.Time
	)

	lastJSON = formatStatus(transport.AgentStatus{Phase: transport.PhaseThinking})
	lastEditTime = time.Now()

	defer func() {
		if flushTimer != nil {
			flushTimer.Stop()
		}
		if thinkTimer != nil {
			thinkTimer.Stop()
		}
	}()

	cancelThinking := func() {
		if thinkTimer != nil {
			thinkTimer.Stop()
			thinkTimer = nil
			thinkCh = nil
		}
	}

	flush := func() {
		if flushTimer != nil {
			flushTimer.Stop()
			flushTimer = nil
			flushCh = nil
		}
		if pending == nil || statusID == "" {
			return
		}
		newJSON := formatStatus(*pending)
		if newJSON != lastJSON {
			_ = r.SendTyping(ctx)
			r.updateStatus(ctx, statusID, *pending)
			lastJSON = newJSON
			lastEditTime = time.Now()
		}
		pending = nil
	}

	schedule := func(s transport.AgentStatus) {
		pending = &s
		elapsed := time.Since(lastEditTime)
		if elapsed >= minEditInterval {
			flush()
			return
		}
		if flushTimer == nil {
			flushTimer = time.NewTimer(minEditInterval - elapsed)
			flushCh = flushTimer.C
		}
	}

	restoreStatus := func() string {
		if statusID == "" {
			return ""
		}
		return r.sendStatus(ctx, transport.AgentStatus{
			Phase: transport.PhaseWorking,
			Plan:  currentPlan,
		})
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return ""
			}

			switch ev.Kind {
			case transport.LLMStart:
				if hadTool {
					cancelThinking()
					thinkTimer = time.NewTimer(thinkingDelay)
					thinkCh = thinkTimer.C
				}

			case transport.LLMStream:
				if d, ok := ev.Data.(transport.LLMStreamData); ok {
					finalText = d.Text
				}

			case transport.ToolStart:
				hadTool = true
				cancelThinking()
				d, _ := ev.Data.(transport.ToolStartData)
				schedule(buildToolStatus(d, currentPlan))

			case transport.Progress:
				cancelThinking()
				d, _ := ev.Data.(transport.ProgressData)
				if d.Tasks != nil {
					currentPlan = d.Tasks
				}
				schedule(transport.AgentStatus{
					Phase:   transport.PhaseWorking,
					Plan:    currentPlan,
					Detail:  d.Status,
					Percent: d.Percent,
				})

			case transport.Message:
				cancelThinking()
				flush()
				d, _ := ev.Data.(transport.MessageData)
				_ = r.SendText(ctx, formatMessageWithLevel(d.Text, d.Level))

			case transport.Ask:
				flush()
				d, _ := ev.Data.(transport.AskData)
				if statusID != "" {
					r.deleteStatus(ctx, statusID)
				}
				_ = r.SendAsk(ctx, d.Prompt, d.Kind, d.Options)

				if replyInbox != nil {
					select {
					case resp := <-replyInbox:
						if d.ReplyCh != nil {
							d.ReplyCh <- resp
						}
					case <-ctx.Done():
						if d.ReplyCh != nil {
							close(d.ReplyCh)
						}
						return ""
					}
				} else {
					if d.ReplyCh != nil {
						d.ReplyCh <- "(no interactive input available)"
					}
				}
				statusID = restoreStatus()
				lastJSON = ""
				lastEditTime = time.Time{}

			case transport.Error:
				if ctx.Err() != nil {
					return ""
				}
				d, _ := ev.Data.(transport.ErrorData)
				if statusID != "" {
					r.deleteStatus(ctx, statusID)
				}
				_ = r.SendText(ctx, "Error: "+d.Error)
				return ""

			case transport.Done:
				if ctx.Err() != nil {
					r.deleteStatus(context.Background(), statusID)
					return ""
				}
				if finalText != "" {
					r.editToFinal(context.Background(), statusID, finalText)
				} else {
					r.deleteStatus(context.Background(), statusID)
				}
				return ""
			}

		case <-flushCh:
			flush()

		case <-thinkCh:
			thinkTimer = nil
			thinkCh = nil
			schedule(transport.AgentStatus{
				Phase:  transport.PhasePlanning,
				Detail: "Planning next moves",
				Plan:   currentPlan,
			})
		}
	}
}
