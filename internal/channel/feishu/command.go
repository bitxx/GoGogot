package feishu

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"gogogot/internal/core/transport"
	"gogogot/internal/tools/store"
	"strings"
)

var commandMap = map[string]string{
	"/new":     channel.CmdNewChat,
	"/stop":    channel.CmdStop,
	"/history": channel.CmdHistory,
	"/memory":  channel.CmdMemory,
	"/soul":    channel.CmdSoul,
	"/user":    channel.CmdUser,
}

func (c *Channel) handleCommand(ctx context.Context, chatID string, reply transport.Replier, cmdText string) {
	if cmdText == "/help" {
		c.sendHelp(ctx, chatID)
		return
	}

	name, ok := commandMap[cmdText]
	if !ok {
		_ = reply.SendText(ctx, "Unknown command. Try /help")
		return
	}

	cmd := &channel.Command{Name: name, Result: &channel.CommandResult{}}
	c.handler(ctx, channel.Message{Reply: reply, Command: cmd})

	if cmd.Result.Error != nil {
		_ = reply.SendText(ctx, "Error: "+cmd.Result.Error.Error())
		return
	}

	if text := formatPayload(cmd.Result.Payload); text != "" {
		_ = reply.SendText(ctx, text)
		return
	}

	if text := cmd.Result.Data["text"]; text != "" {
		_ = reply.SendText(ctx, text)
	}
}

func (c *Channel) sendHelp(ctx context.Context, chatID string) {
	help := "*Commands:*\n" +
		"/new — start a fresh conversation\n" +
		"/history — view past conversations\n" +
		"/memory — list memory files\n" +
		"/soul — show agent identity\n" +
		"/user — show user profile\n" +
		"/stop — cancel the current task\n" +
		"/help — show this help"
	_ = c.newReplier(chatID).SendText(ctx, help)
}

func formatPayload(payload any) string {
	switch v := payload.(type) {
	case []store.ChatInfo:
		return formatHistory(v)
	case []store.MemoryFile:
		return formatMemory(v)
	default:
		return ""
	}
}

func formatHistory(chats []store.ChatInfo) string {
	var closed []store.ChatInfo
	for _, ch := range chats {
		if ch.Status == "closed" {
			closed = append(closed, ch)
		}
	}
	if len(closed) == 0 {
		return "No conversation history yet."
	}

	const maxShown = 15
	if len(closed) > maxShown {
		closed = closed[:maxShown]
	}

	var sb strings.Builder
	sb.WriteString("📜 **Conversation history:**\n\n")
	for _, ch := range closed {
		title := ch.Title
		if title == "" {
			title = "Untitled"
		}
		date := ch.StartedAt.Format("02 Jan")
		if !ch.EndedAt.IsZero() && ch.EndedAt.Format("02 Jan") != date {
			date += " — " + ch.EndedAt.Format("02 Jan")
		}
		fmt.Fprintf(&sb, "**%s** (%s)\n", title, date)
		if ch.Summary != "" {
			fmt.Fprintf(&sb, "_%s_\n", truncateRunes(ch.Summary, 120))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func formatMemory(files []store.MemoryFile) string {
	if len(files) == 0 {
		return "Memory is empty."
	}

	var sb strings.Builder
	for _, f := range files {
		fmt.Fprintf(&sb, "📂 **%s**\n\n%s\n\n---\n", f.Name, strings.TrimSpace(f.Content))
	}
	return sb.String()
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
