package feishu

import (
	"context"
	"encoding/json"
	"gogogot/internal/channel"
	"gogogot/internal/channel/feishu/client"
	"strings"

	"github.com/rs/zerolog/log"
)

// defaultHandler is the EventHandler passed to client.New.
// It decodes the Feishu event envelope and routes to the appropriate handler.
func (c *Channel) defaultHandler(ctx context.Context, env *client.EventEnvelope) {
	if env.Header == nil || env.Event == nil {
		// v1 schema or unrecognised — ignore.
		return
	}

	switch env.Header.EventType {
	case "im.message.receive_v1":
		c.handleMessageEvent(ctx, env)
	case "im.message.message_read_v1":
		// read receipts — ignore
	default:
		log.Trace().Str("type", env.Header.EventType).Msg("feishu: unhandled event type")
	}
}

func (c *Channel) handleMessageEvent(ctx context.Context, env *client.EventEnvelope) {
	ev := env.Event
	if ev.Sender == nil || ev.Message == nil {
		return
	}

	// Only handle messages from the owner.
	senderOpenID := ""
	if ev.Sender.SenderID != nil {
		senderOpenID = ev.Sender.SenderID.OpenID
	}
	if senderOpenID != c.ownerID {
		log.Trace().Str("sender", senderOpenID).Msg("feishu: ignoring message from non-owner")
		return
	}

	msg := ev.Message

	// Determine the reply target:
	// - In a P2P chat, the chat_id is the conversation.
	// - We always reply to the chat (not to the sender's open_id directly)
	//   so group-bot scenarios also work.
	chatID := msg.ChatID

	// Parse the content JSON to extract text / files.
	c.convertAndDispatch(ctx, chatID, msg)
}

// handleCardAction is called when the user clicks a button in an interactive card.
func (c *Channel) handleCardAction(ctx context.Context, chatID, value string) {
	c.handler(ctx, channel.Message{
		Text:  value,
		Reply: c.newReplier(chatID),
	})
}

// ---------------------------------------------------------------------------
// Content type helpers
// ---------------------------------------------------------------------------

type textContent struct {
	Text string `json:"text"`
}

type imageContent struct {
	ImageKey string `json:"image_key"`
}

type fileContent struct {
	FileKey  string `json:"file_key"`
	FileName string `json:"file_name"`
}

type audioContent struct {
	FileKey  string `json:"file_key"`
	Duration int    `json:"duration"`
}

type videoContent struct {
	FileKey   string `json:"file_key"`
	ImageKey  string `json:"image_key"`
	Duration  int    `json:"duration"`
}

type stickerContent struct {
	FileKey string `json:"file_key"`
}

func parseText(content string) string {
	var tc textContent
	if err := json.Unmarshal([]byte(content), &tc); err != nil {
		return ""
	}
	// Feishu may include @mentions as <at user_id="...">Name</at> — strip them.
	text := tc.Text
	for strings.Contains(text, "<at") {
		start := strings.Index(text, "<at")
		end := strings.Index(text[start:], ">")
		if end < 0 {
			break
		}
		closeTag := strings.Index(text[start+end:], "</at>")
		if closeTag < 0 {
			break
		}
		name := text[start+end+1 : start+end+closeTag]
		text = text[:start] + "@" + name + text[start+end+closeTag+5:]
	}
	return strings.TrimSpace(text)
}
