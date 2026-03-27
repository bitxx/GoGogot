package feishu

import (
	"context"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/rs/zerolog/log"

	"gogogot/internal/channel"
)

// defaultHandler is the OnP2MessageReceiveV1 callback registered in Run.
func (c *Channel) defaultHandler(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	if event.Event == nil || event.Event.Sender == nil || event.Event.Message == nil {
		return
	}

	// Only process messages from the owner.
	senderID := event.Event.Sender.SenderId
	if senderID == nil || strVal(senderID.OpenId) != c.ownerID {
		log.Trace().Msg("feishu: ignoring message from non-owner")
		return
	}

	msg := event.Event.Message
	chatID := strVal(msg.ChatId)
	if chatID == "" {
		return
	}

	c.convertAndDispatch(ctx, chatID, msg)
}

// handleCallback is called when the owner clicks a button in an interactive card.
// value is the string stored in the button's "value" field.
func (c *Channel) handleCallback(ctx context.Context, chatID, value string) {
	c.handler(ctx, channel.Message{
		Text:  value,
		Reply: c.newReplier(chatID),
	})
}

// strVal safely dereferences a *string from the SDK.
func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
