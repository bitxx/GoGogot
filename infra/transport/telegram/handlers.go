package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gogogot/core/store"
	"gogogot/infra/transport"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"
)

func (t *Transport) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery, handler transport.Handler) {
	if cb.From.ID != t.ownerID {
		return
	}

	data := cb.Data
	if !strings.HasPrefix(data, "switch_chat:") {
		return
	}

	sofieID := strings.TrimPrefix(data, "switch_chat:")
	chatID := cb.Message.Chat.ID
	channelID := fmt.Sprintf("tg_%d", chatID)

	cmd := &transport.Command{
		Name:   transport.CmdSwitchChat,
		Args:   map[string]string{"chat_id": sofieID},
		Result: &transport.CommandResult{},
	}
	handler(ctx, transport.Message{ChannelID: channelID, Command: cmd})
	if cmd.Result.Error != nil {
		answer := tgbotapi.NewCallback(cb.ID, "Error: "+cmd.Result.Error.Error())
		_, _ = t.api.Request(answer)
		return
	}

	title := cmd.Result.Data["title"]

	answer := tgbotapi.NewCallback(cb.ID, "Switched to: "+title)
	_, _ = t.api.Request(answer)

	text := fmt.Sprintf("✅ Switched to: *%s*", escapeMarkdown(title))
	t.editMessage(chatID, cb.Message.MessageID, text)
}

func (t *Transport) handleMediaGroup(ctx context.Context, msg *tgbotapi.Message, handler transport.Handler) {
	t.mu.Lock()
	defer t.mu.Unlock()

	groupID := msg.MediaGroupID
	if buf, ok := t.mediaGroups[groupID]; ok {
		buf.messages = append(buf.messages, msg)
		buf.timer.Reset(1 * time.Second)
	} else {
		buf := &mediaGroupBuffer{
			messages: []*tgbotapi.Message{msg},
		}
		buf.timer = time.AfterFunc(1*time.Second, func() {
			t.mu.Lock()
			msgs := t.mediaGroups[groupID].messages
			delete(t.mediaGroups, groupID)
			t.mu.Unlock()

			t.convertAndDispatch(ctx, msgs, handler)
		})
		t.mediaGroups[groupID] = buf
	}
}

func (t *Transport) convertAndDispatch(ctx context.Context, msgs []*tgbotapi.Message, handler transport.Handler) {
	if len(msgs) == 0 {
		return
	}

	chatID := msgs[0].Chat.ID
	channelID := fmt.Sprintf("tg_%d", chatID)
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

		if msg.Animation != nil {
			att, err := t.processAnimation(msg.Animation)
			if err != nil {
				log.Error().Err(err).Msg("failed to process animation")
			} else if att != nil {
				attachments = append(attachments, *att)
			}
		} else if msg.Document != nil {
			att, err := t.processDocument(msg.Document)
			if err != nil {
				log.Error().Err(err).Msg("failed to process document")
			} else {
				attachments = append(attachments, att...)
			}
		}

		if len(msg.Photo) > 0 {
			att, err := t.processPhoto(msg.Photo)
			if err != nil {
				log.Error().Err(err).Msg("failed to process photo")
			} else {
				attachments = append(attachments, *att)
			}
		}

		if msg.Audio != nil {
			att, err := t.processAudio(msg.Audio)
			if err != nil {
				log.Error().Err(err).Msg("failed to process audio")
			} else if att != nil {
				attachments = append(attachments, *att)
			}
		}

		if msg.Voice != nil {
			att, err := t.processVoice(msg.Voice)
			if err != nil {
				log.Error().Err(err).Msg("failed to process voice")
			} else if att != nil {
				attachments = append(attachments, *att)
			}
		}

		if msg.Video != nil {
			att, err := t.processVideo(msg.Video)
			if err != nil {
				log.Error().Err(err).Msg("failed to process video")
			} else if att != nil {
				attachments = append(attachments, *att)
			}
		}

		if msg.VideoNote != nil {
			att, err := t.processVideoNote(msg.VideoNote)
			if err != nil {
				log.Error().Err(err).Msg("failed to process video note")
			} else if att != nil {
				attachments = append(attachments, *att)
			}
		}

		if msg.Sticker != nil {
			att, err := t.processSticker(msg.Sticker)
			if err != nil {
				log.Error().Err(err).Msg("failed to process sticker")
			} else if att != nil {
				attachments = append(attachments, *att)
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
		Str("from", msgs[0].From.UserName).
		Msg("telegram incoming message")

	if strings.HasPrefix(text, "/") {
		cmdName := strings.Fields(text)[0]
		if cmdName == "/stop" {
			cmd := &transport.Command{Name: transport.CmdStop, Result: &transport.CommandResult{}}
			handler(ctx, transport.Message{ChannelID: channelID, Command: cmd})
			return
		}
		log.Info().Str("cmd", text).Msg("command received")
		t.handleCommand(ctx, chatID, channelID, text, handler)
		return
	}

	handler(ctx, transport.Message{
		ChannelID:   channelID,
		Text:        text,
		Attachments: attachments,
	})
}

func (t *Transport) handleCommand(ctx context.Context, chatID int64, channelID, text string, handler transport.Handler) {
	parts := strings.Fields(text)
	cmdText := parts[0]

	switch cmdText {
	case "/start", "/new":
		cmd := &transport.Command{Name: transport.CmdNewChat, Result: &transport.CommandResult{}}
		handler(ctx, transport.Message{ChannelID: channelID, Command: cmd})
		if cmd.Result.Error != nil {
			t.send(chatID, "Error: "+escapeMarkdown(cmd.Result.Error.Error()))
			return
		}
		t.send(chatID, "✨ New chat started\\.")

	case "/help":
		t.send(chatID, "*Commands:*\n"+
			"/new — start a fresh chat\n"+
			"/chats — list and switch chats\n"+
			"/memory — list memory files\n"+
			"/stop — cancel the current task\n"+
			"/help — show this help")

	case "/chats":
		chats, err := store.ListChats()
		if err != nil {
			t.send(chatID, "Error: "+escapeMarkdown(err.Error()))
			return
		}
		if len(chats) == 0 {
			t.send(chatID, "No chats yet\\. Send a message to start one\\!")
			return
		}

		currentID, _ := store.GetExternalMapping(channelID)

		const maxChats = 20
		if len(chats) > maxChats {
			chats = chats[:maxChats]
		}

		var rows [][]tgbotapi.InlineKeyboardButton
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
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, "switch_chat:"+c.ID),
			))
		}

		msg := tgbotapi.NewMessage(chatID, "💬 Your chats:")
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
		if _, err := t.api.Send(msg); err != nil {
			log.Error().Err(err).Msg("telegram send failed")
		}

	case "/memory":
		files, err := store.ListMemory()
		if err != nil {
			t.send(chatID, "Error: "+escapeMarkdown(err.Error()))
			return
		}
		if len(files) == 0 {
			t.send(chatID, "Memory is empty — no files yet\\.")
			return
		}
		var sb strings.Builder
		sb.WriteString("📂 *Memory files:*\n\n")
		for _, f := range files {
			fmt.Fprintf(&sb, "`%s` \\(%d bytes\\)\n", escapeMarkdown(f.Name), f.Size)
		}
		t.send(chatID, sb.String())

	default:
		t.send(chatID, "Unknown command\\. Try /help")
	}
}
