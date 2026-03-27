package feishu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gogogot/internal/core/transport"
)

type replier struct {
	ch     *Channel
	chatID string
}

func (r *replier) SendText(ctx context.Context, text string) error {
	r.ch.sendLong(ctx, r.chatID, text)
	return nil
}

func (r *replier) SendFile(ctx context.Context, path, caption string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	lower := strings.ToLower(path)
	filename := filepath.Base(path)

	if hasSuffix(lower, ".jpg", ".jpeg", ".png", ".webp", ".gif") {
		return r.ch.sendImage(ctx, r.chatID, data, caption)
	}
	return r.ch.sendFile(ctx, r.chatID, data, filename, caption)
}

func (r *replier) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	switch kind {
	case transport.AskConfirm:
		card := buttonCard(prompt, []buttonOpt{
			{"✅ Yes", "yes"},
			{"❌ No", "no"},
		})
		r.ch.sendCard(ctx, r.chatID, card)
		return nil

	case transport.AskChoice:
		var btns []buttonOpt
		for _, opt := range options {
			btns = append(btns, buttonOpt{opt.Label, opt.Value})
		}
		r.ch.sendCard(ctx, r.chatID, buttonCard(prompt, btns))
		return nil

	default:
		r.ch.sendLong(ctx, r.chatID, "❓ "+prompt)
		return nil
	}
}

func (r *replier) SendTyping(ctx context.Context) error {
	// Feishu long-connection has no typing-indicator API — no-op.
	return nil
}
