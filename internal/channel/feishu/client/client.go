package client

import (
	"context"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/rs/zerolog/log"
)

type MessageHandler func(ctx context.Context, event *larkim.P2MessageReceiveV1) error

type Client struct {
	apiClient *lark.Client
	wsClient  *larkws.Client
}

func NewClient(appID, appSecret string, handler MessageHandler) (*Client, error) {
	apiClient := lark.NewClient(appID, appSecret)

	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			return handler(ctx, event)
		})

	wsClient := larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	return &Client{
		apiClient: apiClient,
		wsClient:  wsClient,
	}, nil
}

func (c *Client) Start(ctx context.Context) error {
	log.Info().Msg("starting feishu websocket client...")
	if err := c.wsClient.Start(ctx); err != nil {
		return fmt.Errorf("websocket start failed: %w", err)
	}
	log.Info().Msg("feishu websocket stopped")
	return nil
}

func (c *Client) SendText(ctx context.Context, receiveID, text string) (string, error) {
	content := larkim.NewTextMsgBuilder().
		Text(text).
		Build()

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeOpenId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			MsgType(larkim.MsgTypeText).
			ReceiveId(receiveID).
			Content(content).
			Build()).
		Build()

	resp, err := c.apiClient.Im.Message.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", fmt.Errorf("send failed: %s", resp.Msg)
	}
	return *resp.Data.MessageId, nil
}

func (c *Client) SendReply(ctx context.Context, messageID, text string) error {
	content := larkim.NewTextMsgBuilder().
		Text(text).
		Build()

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType(larkim.MsgTypeText).
			Content(content).
			Build()).
		Build()

	resp, err := c.apiClient.Im.Message.Reply(ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("reply failed: %s", resp.Msg)
	}
	return nil
}
