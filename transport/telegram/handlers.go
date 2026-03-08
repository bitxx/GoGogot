package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gogogot/transport"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

func (t *Transport) handleCallback(ctx context.Context, cb *models.CallbackQuery) {
	if cb.From.ID != t.ownerID {
		return
	}

	data := cb.Data
	if !strings.HasPrefix(data, callbackPrefix) {
		return
	}

	sofieID := strings.TrimPrefix(data, callbackPrefix)

	var chatID int64
	var messageID int
	if cb.Message.Message != nil {
		chatID = cb.Message.Message.Chat.ID
		messageID = cb.Message.Message.ID
	} else if cb.Message.InaccessibleMessage != nil {
		chatID = cb.Message.InaccessibleMessage.Chat.ID
		messageID = cb.Message.InaccessibleMessage.MessageID
	} else {
		return
	}
	channelID := fmt.Sprintf("%s%d", channelPrefix, chatID)

	cmd := &transport.Command{
		Name:   transport.CmdSwitchChat,
		Args:   map[string]string{"chat_id": sofieID},
		Result: &transport.CommandResult{},
	}
	t.handler(ctx, transport.Message{ChannelID: channelID, Command: cmd})
	if cmd.Result.Error != nil {
		t.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            "Error: " + cmd.Result.Error.Error(),
		})
		return
	}

	title := cmd.Result.Data["title"]

	t.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            "Switched to: " + title,
	})

	text := fmt.Sprintf("✅ Switched to: *%s*", bot.EscapeMarkdown(title))
	t.editMessage(ctx, chatID, messageID, text)
}

func (t *Transport) handleMediaGroup(ctx context.Context, msg *models.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()

	groupID := msg.MediaGroupID
	if buf, ok := t.mediaGroups[groupID]; ok {
		buf.messages = append(buf.messages, msg)
		buf.timer.Reset(1 * time.Second)
	} else {
		buf := &mediaGroupBuffer{
			messages: []*models.Message{msg},
		}
		buf.timer = time.AfterFunc(1*time.Second, func() {
			if ctx.Err() != nil {
				return
			}
			t.mu.Lock()
			msgs := t.mediaGroups[groupID].messages
			delete(t.mediaGroups, groupID)
			t.mu.Unlock()

			t.convertAndDispatch(ctx, msgs)
		})
		t.mediaGroups[groupID] = buf
	}
}

type mediaExtractor struct {
	check   func(*models.Message) bool
	process func(t *Transport, ctx context.Context, msg *models.Message) ([]transport.Attachment, error)
}

var mediaExtractors = []mediaExtractor{
	{
		check: func(m *models.Message) bool { return m.Animation != nil },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processAnimation(ctx, m.Animation)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Document != nil && m.Animation == nil },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processDocument(ctx, m.Document)
		},
	},
	{
		check: func(m *models.Message) bool { return len(m.Photo) > 0 },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processPhoto(ctx, m.Photo)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Audio != nil },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processAudio(ctx, m.Audio)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Voice != nil },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processVoice(ctx, m.Voice)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Video != nil },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processVideo(ctx, m.Video)
		},
	},
	{
		check: func(m *models.Message) bool { return m.VideoNote != nil },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processVideoNote(ctx, m.VideoNote)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Sticker != nil },
		process: func(t *Transport, ctx context.Context, m *models.Message) ([]transport.Attachment, error) {
			return t.processSticker(ctx, m.Sticker)
		},
	},
}

func (t *Transport) convertAndDispatch(ctx context.Context, msgs []*models.Message) {
	if len(msgs) == 0 {
		return
	}

	chatID := msgs[0].Chat.ID
	channelID := fmt.Sprintf("%s%d", channelPrefix, chatID)
	var textParts []string
	var attachments []transport.Attachment

	for _, msg := range msgs {
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			text = strings.TrimSpace(msg.Caption)
		}
		if text != "" {
			textParts = append(textParts, text)
		}

		for _, ex := range mediaExtractors {
			if ex.check(msg) {
				atts, err := ex.process(t, ctx, msg)
				if err != nil {
					log.Error().Err(err).Msg("failed to process media")
				} else {
					attachments = append(attachments, atts...)
				}
			}
		}

		if msg.Venue != nil {
			textParts = append(textParts, fmt.Sprintf("[Venue: %s, %s — lat=%.6f, lon=%.6f]",
				msg.Venue.Title, msg.Venue.Address,
				msg.Venue.Location.Latitude, msg.Venue.Location.Longitude))
		} else if msg.Location != nil {
			textParts = append(textParts, fmt.Sprintf("[Location: lat=%.6f, lon=%.6f]",
				msg.Location.Latitude, msg.Location.Longitude))
		}

		if msg.Contact != nil {
			textParts = append(textParts, fmt.Sprintf("[Contact: %s %s, phone: %s]",
				msg.Contact.FirstName, msg.Contact.LastName, msg.Contact.PhoneNumber))
		}

		if msg.Poll != nil {
			opts := make([]string, len(msg.Poll.Options))
			for i, o := range msg.Poll.Options {
				opts[i] = o.Text
			}
			textParts = append(textParts, fmt.Sprintf("[Poll: %s — options: %s]",
				msg.Poll.Question, strings.Join(opts, ", ")))
		}

		if msg.Dice != nil {
			textParts = append(textParts, fmt.Sprintf("[Dice: %s = %d]",
				msg.Dice.Emoji, msg.Dice.Value))
		}
	}

	text := strings.Join(textParts, "\n\n")

	var fileTexts []string
	for _, att := range attachments {
		if !strings.HasPrefix(att.MimeType, "image/") && isTextMIME(att.MimeType) {
			fileTexts = append(fileTexts, fmt.Sprintf("[File: %s]\n```\n%s\n```", att.Filename, string(att.Data)))
		}
	}

	if len(fileTexts) > 0 {
		filesStr := strings.Join(fileTexts, "\n\n")
		if text != "" {
			text = filesStr + "\n\n" + text
		} else {
			text = filesStr
		}
	}

	if text == "" && len(attachments) == 0 {
		return
	}

	if text == "" && len(attachments) > 0 {
		text = "What's in these files?"
	}

	log.Debug().
		Int64("chat_id", chatID).
		Int("text_len", len(text)).
		Int("attachments", len(attachments)).
		Msg("telegram incoming message")

	if strings.HasPrefix(text, "/") {
		cmdName := strings.Fields(text)[0]
		if cmdName == "/stop" {
			cmd := &transport.Command{Name: transport.CmdStop, Result: &transport.CommandResult{}}
			t.handler(ctx, transport.Message{ChannelID: channelID, Command: cmd})
			return
		}
		log.Info().Str("cmd", text).Msg("command received")
		t.handleCommand(ctx, chatID, channelID, text)
		return
	}

	t.handler(ctx, transport.Message{
		ChannelID:   channelID,
		Text:        text,
		Attachments: attachments,
	})
}

func (t *Transport) handleCommand(ctx context.Context, chatID int64, channelID, text string) {
	parts := strings.Fields(text)
	cmdText := parts[0]

	switch cmdText {
	case "/start", "/new":
		cmd := &transport.Command{Name: transport.CmdNewChat, Result: &transport.CommandResult{}}
		t.handler(ctx, transport.Message{ChannelID: channelID, Command: cmd})
		if cmd.Result.Error != nil {
			t.send(ctx, chatID, "Error: "+bot.EscapeMarkdown(cmd.Result.Error.Error()))
			return
		}
		t.send(ctx, chatID, "✨ New chat started\\.")

	case "/help":
		t.send(ctx, chatID, "*Commands:*\n"+
			"/new — start a fresh chat\n"+
			"/chats — list and switch chats\n"+
			"/memory — list memory files\n"+
			"/stop — cancel the current task\n"+
			"/help — show this help")

	case "/chats":
		chats, err := t.chatLister.ListChats()
		if err != nil {
			t.send(ctx, chatID, "Error: "+bot.EscapeMarkdown(err.Error()))
			return
		}
		if len(chats) == 0 {
			t.send(ctx, chatID, "No chats yet\\. Send a message to start one\\!")
			return
		}

		currentID, _ := t.chatLister.GetExternalMapping(channelID)

		if len(chats) > maxChatsShown {
			chats = chats[:maxChatsShown]
		}

		var rows [][]models.InlineKeyboardButton
		for _, c := range chats {
			title := c.Title
			if title == "" {
				title = "Untitled"
			}
			if len([]rune(title)) > 40 {
				title = string([]rune(title)[:40]) + "…"
			}
			date := c.UpdatedAt.Format("02 Jan")
			label := fmt.Sprintf("%s — %s", title, date)
			if c.ID == currentID {
				label = "● " + label
			}
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: label, CallbackData: callbackPrefix + c.ID},
			})
		}

		t.b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "💬 Your chats:",
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: rows,
			},
		})

	case "/memory":
		files, err := t.memoryLister.ListMemory()
		if err != nil {
			t.send(ctx, chatID, "Error: "+bot.EscapeMarkdown(err.Error()))
			return
		}
		if len(files) == 0 {
			t.send(ctx, chatID, "Memory is empty — no files yet\\.")
			return
		}
		var sb strings.Builder
		sb.WriteString("📂 *Memory files:*\n\n")
		for _, f := range files {
			fmt.Fprintf(&sb, "`%s` \\(%d bytes\\)\n", bot.EscapeMarkdown(f.Name), f.Size)
		}
		t.send(ctx, chatID, sb.String())

	default:
		t.send(ctx, chatID, "Unknown command\\. Try /help")
	}
}
