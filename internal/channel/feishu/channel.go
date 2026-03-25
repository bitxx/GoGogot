package feishu

import (
	"context"
	"encoding/json"
	"gogogot/internal/channel"
	"gogogot/internal/channel/feishu/client"
	"gogogot/internal/core/transport"
	"sync"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/rs/zerolog/log"
)

type Channel struct {
	client  *client.Client
	ownerID string

	handler channel.Handler

	mu          sync.Mutex
	pendingReqs map[string]chan string
}

func New(appID, appSecret, ownerID string) (*Channel, error) {
	ch := &Channel{
		ownerID:     ownerID,
		pendingReqs: make(map[string]chan string),
	}

	cl, err := client.NewClient(appID, appSecret, ch.handleMessage)
	if err != nil {
		return nil, err
	}
	ch.client = cl

	return ch, nil
}

func (c *Channel) Name() string { return "feishu" }

func (c *Channel) OwnerReplier() transport.Replier {
	return c.newReplier(c.ownerID)
}

func (c *Channel) newReplier(chatID string) *replier {
	return &replier{ch: c, chatID: chatID}
}

func (c *Channel) Run(ctx context.Context, handler channel.Handler) error {
	c.handler = handler
	log.Info().Str("owner_id", c.ownerID).Msg("feishu bot started")
	return c.client.Start(ctx)
}

// handleMessage 处理接收到的消息
// 注意：这里我们不提取发送者 ID，假设所有消息都来自管理员（owner）。
// 如果需要支持多用户，请根据实际事件 JSON 调整。
func (c *Channel) handleMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	msg := event.Event.Message
	if msg == nil {
		log.Warn().Msg("message is nil")
		return nil
	}

	// 只处理文本消息
	if msg.MessageType == nil || *msg.MessageType != "text" {
		return nil
	}

	// 解析 content
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
		log.Error().Err(err).Msg("parse message content failed")
		return nil
	}

	// 使用 ownerID 作为发送者 ID（简化处理）
	senderID := c.ownerID
	text := content.Text

	// 检查是否有等待的 ask 响应
	c.mu.Lock()
	if ch, ok := c.pendingReqs[senderID]; ok {
		delete(c.pendingReqs, senderID)
		c.mu.Unlock()
		select {
		case ch <- text:
		default:
		}
		return nil
	}
	c.mu.Unlock()

	reply := c.newReplier(senderID)

	// 命令处理
	if len(text) > 0 && text[0] == '/' {
		c.handleCommand(ctx, senderID, reply, text)
		return nil
	}

	// 普通消息转发
	c.handler(ctx, channel.Message{
		Text:  text,
		Reply: reply,
	})

	return nil
}
