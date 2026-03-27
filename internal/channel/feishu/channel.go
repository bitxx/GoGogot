package feishu

import (
	"context"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/rs/zerolog/log"

	"gogogot/internal/channel"
	"gogogot/internal/core/transport"
)

// Channel implements channel.Channel for Feishu (Lark).
// It uses the official oapi-sdk-go/v3 long-connection (WebSocket) mode —
// no public HTTP endpoint required.
type Channel struct {
	// api is the Feishu REST client (send/edit/delete messages, upload files).
	api *lark.Client
	// ownerID is the open_id of the user who owns this bot instance.
	ownerID   string
	appID     string
	appSecret string

	handler channel.Handler
}

// New creates a Feishu channel.
//   - appID, appSecret — Feishu app credentials (自建应用)
//   - ownerID          — open_id of the owner; used for OwnerReplier
func New(appID, appSecret, ownerID string) (*Channel, error) {
	api := lark.NewClient(appID, appSecret,
		lark.WithLogLevel(larkcore.LogLevelWarn),
	)
	return &Channel{
		api:       api,
		appID:     appID,
		appSecret: appSecret,
		ownerID:   ownerID,
	}, nil
}

func (c *Channel) Name() string { return "feishu" }

func (c *Channel) OwnerReplier() transport.Replier {
	return c.newReplier(c.ownerID)
}

func (c *Channel) newReplier(chatID string) *replier {
	return &replier{ch: c, chatID: chatID}
}

// Run starts the long-connection listener and blocks until ctx is cancelled.
// handler is called for every incoming owner message / command.
func (c *Channel) Run(ctx context.Context, handler channel.Handler) error {
	c.handler = handler

	// Long-connection mode: verificationToken and encryptKey must both be "".
	eventDispatcher := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			c.defaultHandler(ctx, event)
			return nil
		}).
		OnP2CardActionTrigger(func(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
			return c.handleCardAction(ctx, event)
		})

	wsClient := larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithLogLevel(larkcore.LogLevelWarn),
	)

	log.Info().Str("owner_id", c.ownerID).Msg("feishu long-connection started")
	return wsClient.Start(ctx)
}
