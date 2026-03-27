package feishu

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"gogogot/internal/core/transport"
)

var phaseEmoji = map[transport.Phase]string{
	transport.PhaseThinking: "🧠",
	transport.PhasePlanning: "📋",
	transport.PhaseTool:     "🔧",
	transport.PhaseWorking:  "⚡",
	transport.PhaseMessage:  "💬",
}

var phaseColor = map[transport.Phase]string{
	transport.PhaseThinking: "grey",
	transport.PhasePlanning: "blue",
	transport.PhaseTool:     "yellow",
	transport.PhaseWorking:  "blue",
	transport.PhaseMessage:  "green",
}

// formatStatus builds the card JSON for the given agent status.
// This is the Feishu equivalent of Telegram's formatStatus (which returns
// MarkdownV2 text; here we return a card JSON string).
func formatStatus(s transport.AgentStatus) string {
	var parts []string

	if len(s.Plan) > 0 {
		parts = append(parts, formatPlanLine(s.Plan))
	}
	if s.Percent != nil {
		parts = append(parts, formatProgressBar(*s.Percent))
	}

	emoji := phaseEmoji[s.Phase]
	if emoji == "" {
		emoji = "⏳"
	}
	color := phaseColor[s.Phase]
	if color == "" {
		color = "grey"
	}

	label := s.Detail
	if label == "" {
		switch s.Phase {
		case transport.PhaseThinking:
			label = "Thinking"
		case transport.PhasePlanning:
			label = "Planning"
		default:
			label = string(s.Phase)
		}
	}

	header := emoji + " " + label
	if s.Phase != transport.PhaseMessage {
		header += "..."
	}

	return headerCard(header, color, strings.Join(parts, "\n"))
}

func formatProgressBar(pct int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	const width = 10
	filled := pct * width / 100
	return fmt.Sprintf("`%s%s` %d%%",
		strings.Repeat("█", filled),
		strings.Repeat("░", width-filled),
		pct)
}

func formatPlanLine(tasks []transport.PlanTask) string {
	completed := 0
	var activeTitle string
	var icons []string
	for _, t := range tasks {
		switch t.Status {
		case transport.TaskCompleted:
			completed++
			icons = append(icons, "✅")
		case transport.TaskInProgress:
			icons = append(icons, "▸")
			if activeTitle == "" {
				activeTitle = t.Title
			}
		default:
			icons = append(icons, "○")
		}
	}
	line := fmt.Sprintf("📋 %d/%d %s", completed, len(tasks), strings.Join(icons, ""))
	if activeTitle != "" {
		line += "\n▸ " + activeTitle
	}
	return line
}

// ---------------------------------------------------------------------------
// replier status methods — mirror Telegram's sendStatus / updateStatus /
// deleteStatus / editToFinal exactly, using card message_id as the handle.
// ---------------------------------------------------------------------------

func (r *replier) sendStatus(ctx context.Context, status transport.AgentStatus) string {
	return r.ch.sendCard(ctx, r.chatID, formatStatus(status))
}

func (r *replier) updateStatus(ctx context.Context, msgID string, status transport.AgentStatus) {
	if msgID == "" {
		return
	}
	if err := r.ch.patchMsg(ctx, msgID, formatStatus(status)); err != nil {
		log.Warn().Err(err).Str("msg_id", msgID).Str("phase", string(status.Phase)).
			Msg("feishu: updateStatus patchMsg failed")
	}
}

func (r *replier) deleteStatus(ctx context.Context, msgID string) {
	if msgID == "" {
		return
	}
	if err := r.ch.deleteMsg(ctx, msgID); err != nil {
		log.Warn().Err(err).Str("msg_id", msgID).Msg("feishu: deleteStatus failed")
	}
}

// editToFinal replaces the status card with the final response text.
// If the text fits in one card it edits in-place; otherwise it deletes the
// status card and sends the full text as chunked card messages.
func (r *replier) editToFinal(ctx context.Context, msgID, text string) {
	if msgID == "" {
		r.ch.sendCardLong(ctx, r.chatID, text)
		return
	}

	chunks := splitText(text, maxCardLen)
	if len(chunks) == 0 {
		r.deleteStatus(ctx, msgID)
		return
	}

	// Single chunk: patch in-place.
	if len(chunks) == 1 {
		if err := r.ch.patchMsg(ctx, msgID, markdownCard(chunks[0])); err != nil {
			log.Warn().Err(err).Msg("feishu: editToFinal patch failed, falling back to delete+send")
			r.deleteStatus(ctx, msgID)
			r.ch.sendCardLong(ctx, r.chatID, text)
		}
		return
	}

	// Multiple chunks: patch status with first, send rest as new messages.
	if err := r.ch.patchMsg(ctx, msgID, markdownCard(chunks[0])); err != nil {
		log.Warn().Err(err).Msg("feishu: editToFinal patch failed, falling back to delete+send")
		r.deleteStatus(ctx, msgID)
		r.ch.sendCardLong(ctx, r.chatID, text)
		return
	}
	for _, chunk := range chunks[1:] {
		if r.ch.sendCard(ctx, r.chatID, markdownCard(chunk)) == "" {
			r.ch.sendText(ctx, r.chatID, chunk)
		}
	}
}
