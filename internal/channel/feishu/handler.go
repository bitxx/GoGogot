package feishu

import (
	"context"
	"encoding/json"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/rs/zerolog/log"

	"gogogot/internal/channel"
)

// defaultHandler is the OnP2MessageReceiveV1 callback registered in Run.
func (c *Channel) defaultHandler(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	if event.Event == nil || event.Event.Sender == nil || event.Event.Message == nil {
		return
	}

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

// handleCardAction is called when the owner clicks a button in an interactive
// card. It mirrors Telegram's handleCallback exactly.
func (c *Channel) handleCardAction(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if event.Event == nil || event.Event.Action == nil {
		return nil, nil
	}

	// Only handle actions from the owner.
	operatorID := event.Event.Operator
	if operatorID == nil || operatorID.OpenID != c.ownerID {
		return nil, nil
	}

	// Extract the "value" key that buttonCard embeds in every button.
	action := event.Event.Action
	if action == nil || action.Value == nil {
		return nil, nil
	}
	raw, _ := json.Marshal(action.Value)
	var val struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &val); err != nil || val.Value == "" {
		return nil, err
	}

	// Resolve chat_id: prefer the message's open_chat_id, fall back to owner.
	chatID := c.ownerID
	if event.Event.Context != nil && event.Event.Context.OpenChatID != "" {
		chatID = event.Event.Context.OpenChatID
	}

	c.handler(ctx, channel.Message{
		Text:  val.Value,
		Reply: c.newReplier(chatID),
	})
	return nil, nil
}

// strVal safely dereferences a *string from the SDK.
func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
