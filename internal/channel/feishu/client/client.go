package client

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/rs/zerolog/log"
)

// MessageHandler 消息处理函数类型，对应长代码中的 core.MessageHandler
type MessageHandler func(ctx context.Context, event *larkim.P2MessageReceiveV1) error

// Client 飞书机器人客户端，对应长代码中的 Platform
type Client struct {
	appID        string
	appSecret    string
	domain       string
	useWebSocket bool
	client       *lark.Client
	eventHandler *dispatcher.EventDispatcher
	wsClient     *larkws.Client
	server       *http.Server
	cancel       context.CancelFunc
	mu           sync.Mutex
}

// New 创建飞书客户端，对应长代码中的 newPlatform 和 New
// domain: API域名，国内用 lark.FeishuBaseUrl，国际用 lark.LarkBaseUrl
// useWebSocket: true 使用 WebSocket（飞书国内版），false 使用 Webhook（Lark国际版）
// handler: 消息处理函数
func New(appID, appSecret, domain string, useWebSocket bool, handler MessageHandler) (*Client, error) {
	if domain == "" {
		domain = lark.FeishuBaseUrl
	}
	client := lark.NewClient(appID, appSecret, lark.WithOpenBaseUrl(domain))

	// 事件分发器，只注册消息接收事件（长代码中的 OnP2MessageReceiveV1）
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(handler)

	c := &Client{
		appID:        appID,
		appSecret:    appSecret,
		domain:       domain,
		useWebSocket: useWebSocket,
		client:       client,
		eventHandler: eventHandler,
	}
	log.Info().Msg("feishu/lark client created")
	return c, nil
}

// Start 启动服务，对应长代码中的 Start 方法
// port: Webhook 模式下监听的端口，WebSocket 模式下忽略
func (c *Client) Start(ctx context.Context, port string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	if c.useWebSocket {
		// 飞书国内版：WebSocket 长连接（长代码中的 startWebSocketMode）
		c.wsClient = larkws.NewClient(c.appID, c.appSecret,
			larkws.WithEventHandler(c.eventHandler),
			larkws.WithLogLevel(larkcore.LogLevelInfo),
			larkws.WithDomain(c.domain),
		)
		go func() {
			if err := c.wsClient.Start(ctx); err != nil {
				log.Error().Err(err).Msg("feishu websocket error")
			}
		}()
		log.Info().Msg("feishu websocket started")
	} else {
		// Lark 国际版：HTTP Webhook 服务器（长代码中的 startWebhookMode）
		mux := http.NewServeMux()
		mux.HandleFunc("/webhook", c.webhookHandler)
		c.server = &http.Server{
			Addr:    ":" + port,
			Handler: mux,
		}
		go func() {
			if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("lark webhook server error")
			}
		}()
		log.Info().Str("port", port).Msg("lark webhook server started")
	}
}

// Stop 停止服务，对应长代码中的 Stop 方法
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return c.server.Shutdown(ctx)
	}
	return nil
}

// webhookHandler 处理 HTTP 回调请求，对应长代码中的 webhookHandler
func (c *Client) webhookHandler(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		defer r.Body.Close()
		body, _ = io.ReadAll(r.Body)
	}
	eventReq := &larkevent.EventReq{
		Header:     r.Header,
		Body:       body,
		RequestURI: r.RequestURI,
	}
	resp := c.eventHandler.Handle(r.Context(), eventReq)
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	if len(resp.Body) > 0 {
		w.Write(resp.Body)
	}
}
