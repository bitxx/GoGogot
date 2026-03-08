package telegram

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

func (t *Transport) send(ctx context.Context, chatID int64, markdownText string) {
	_, err := t.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      markdownText,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		log.Error().Err(err).Msg("telegram send failed")
	}
}

func (t *Transport) sendAndGetID(ctx context.Context, chatID int64, markdownText string) int {
	sent, err := t.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      markdownText,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		log.Error().Err(err).Msg("telegram send failed")
		return 0
	}
	return sent.ID
}

func (t *Transport) sendLong(ctx context.Context, chatID int64, text string) {
	for _, chunk := range FormatHTMLChunks(text, maxMessageLen) {
		sent, err := t.b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      chunk.HTML,
			ParseMode: models.ParseModeHTML,
		})
		_ = sent
		if err != nil {
			log.Warn().Err(err).Msg("telegram HTML send failed, falling back to plain text")
			_, err = t.b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   chunk.Text,
			})
			if err != nil {
				log.Error().Err(err).Msg("telegram plain text send failed")
			}
		}
	}
}

func (t *Transport) editMessage(ctx context.Context, chatID int64, messageID int, markdownText string) {
	if messageID == 0 {
		return
	}
	_, err := t.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      markdownText,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		log.Debug().Err(err).Msg("telegram edit failed")
	}
}

func (t *Transport) deleteMessage(ctx context.Context, chatID int64, messageID int) {
	if messageID == 0 {
		return
	}
	_, err := t.b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	if err != nil {
		log.Debug().Err(err).Msg("telegram delete failed")
	}
}
