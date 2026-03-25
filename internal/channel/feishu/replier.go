package feishu

import (
	"context"
	"gogogot/internal/core/transport"
	"sync"
)

type replier struct {
	ch     *Channel
	chatID string
}

func (r *replier) SendText(ctx context.Context, text string) error {
	_, err := r.ch.client.SendText(ctx, r.chatID, text)
	return err
}

func (r *replier) SendFile(ctx context.Context, path, caption string) error {
	return r.SendText(ctx, "[File: "+path+"]\n"+caption)
}

func (r *replier) SendTyping(ctx context.Context) error {
	return nil
}

func (r *replier) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	text := "❓ " + prompt
	if kind == transport.AskConfirm {
		text += "\n（请回复 是/否）"
	} else if kind == transport.AskChoice && len(options) > 0 {
		text += "\n选项："
		for _, opt := range options {
			text += "\n- " + opt.Label
		}
	}

	respCh := make(chan string, 1)
	r.ch.mu.Lock()
	r.ch.pendingReqs[r.chatID] = respCh
	r.ch.mu.Unlock()

	if err := r.SendText(ctx, text); err != nil {
		r.ch.mu.Lock()
		delete(r.ch.pendingReqs, r.chatID)
		r.ch.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		r.ch.mu.Lock()
		delete(r.ch.pendingReqs, r.chatID)
		r.ch.mu.Unlock()
		return ctx.Err()
	case <-respCh:
		return nil
	}
}

func (r *replier) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	var finalText string
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case resp, ok := <-replyInbox:
				if !ok {
					return
				}
				r.ch.mu.Lock()
				if ch, exists := r.ch.pendingReqs[r.chatID]; exists {
					delete(r.ch.pendingReqs, r.chatID)
					ch <- resp
				}
				r.ch.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				wg.Wait()
				return finalText
			}

			switch ev.Kind {
			case transport.LLMStream:
				if d, ok := ev.Data.(transport.LLMStreamData); ok {
					finalText = d.Text
				}
			case transport.Message:
				d, _ := ev.Data.(transport.MessageData)
				_ = r.SendText(ctx, formatMessageWithLevel(d.Text, d.Level))
			case transport.Error:
				d, _ := ev.Data.(transport.ErrorData)
				_ = r.SendText(ctx, "Error: "+d.Error)
				wg.Wait()
				return ""
			case transport.Done:
				if finalText != "" {
					_ = r.SendText(ctx, finalText)
				}
				wg.Wait()
				return finalText
			case transport.Ask:
				d, _ := ev.Data.(transport.AskData)
				_ = r.SendAsk(ctx, d.Prompt, d.Kind, d.Options)
			default:
			}
		case <-ctx.Done():
			wg.Wait()
			return ""
		}
	}
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
