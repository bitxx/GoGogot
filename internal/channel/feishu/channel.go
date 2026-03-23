package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"gogogot/internal/core/transport"
	"gogogot/internal/platform/feishu"
)

// FeishuChannel 实现 Channel 接口，适配飞书平台。
type FeishuChannel struct {
	name       string
	platform   *feishu.Platform
	owner      transport.Replier // 预构建的 owner replier
	cfg        *feishuConfig
	msgHandler Handler
	mu         sync.Mutex
}

type feishuConfig struct {
	appID                 string
	appSecret             string
	ownerChatID           string // 用于 OwnerReplier 的目标 chat_id
	ownerUserID           string // 可选，优先级低于 ownerChatID
	allowFrom             string
	groupReplyAll         bool
	shareSessionInChannel bool
	threadIsolation       bool
	reactionEmoji         string
	port                  string
	callbackPath          string
	encryptKey            string
	enableCard            bool
}

// NewFeishuChannel 根据配置创建飞书通道。
func NewFeishuChannel(opts map[string]any) (*FeishuChannel, error) {
	cfg := &feishuConfig{}
	cfg.appID, _ = opts["app_id"].(string)
	cfg.appSecret, _ = opts["app_secret"].(string)
	if cfg.appID == "" || cfg.appSecret == "" {
		return nil, fmt.Errorf("feishu: app_id and app_secret are required")
	}
	cfg.ownerChatID, _ = opts["owner_chat_id"].(string)
	cfg.ownerUserID, _ = opts["owner_user_id"].(string)
	cfg.allowFrom, _ = opts["allow_from"].(string)
	cfg.groupReplyAll, _ = opts["group_reply_all"].(bool)
	cfg.shareSessionInChannel, _ = opts["share_session_in_channel"].(bool)
	cfg.threadIsolation, _ = opts["thread_isolation"].(bool)
	cfg.reactionEmoji, _ = opts["reaction_emoji"].(string)
	if cfg.reactionEmoji == "" {
		cfg.reactionEmoji = "OnIt"
	}
	cfg.port, _ = opts["port"].(string)
	if cfg.port == "" {
		cfg.port = "8080"
	}
	cfg.callbackPath, _ = opts["callback_path"].(string)
	if cfg.callbackPath == "" {
		cfg.callbackPath = "/feishu/webhook"
	}
	cfg.encryptKey, _ = opts["encrypt_key"].(string)
	cfg.enableCard = true
	if v, ok := opts["enable_feishu_card"].(bool); ok {
		cfg.enableCard = v
	}

	platformOpts := map[string]any{
		"app_id":                   cfg.appID,
		"app_secret":               cfg.appSecret,
		"allow_from":               cfg.allowFrom,
		"group_reply_all":          cfg.groupReplyAll,
		"share_session_in_channel": cfg.shareSessionInChannel,
		"thread_isolation":         cfg.threadIsolation,
		"reaction_emoji":           cfg.reactionEmoji,
		"port":                     cfg.port,
		"callback_path":            cfg.callbackPath,
		"encrypt_key":              cfg.encryptKey,
		"enable_feishu_card":       cfg.enableCard,
	}
	platform, err := feishu.New(platformOpts)
	if err != nil {
		return nil, fmt.Errorf("feishu: create platform: %w", err)
	}

	ch := &FeishuChannel{
		name:     "feishu",
		platform: platform,
		cfg:      cfg,
	}
	ch.owner = ch.newReplierForOwner()
	return ch, nil
}

// Name 返回通道名称。
func (c *FeishuChannel) Name() string {
	return c.name
}

// OwnerReplier 返回一个可以向配置的 owner 发送消息的 Replier。
func (c *FeishuChannel) OwnerReplier() transport.Replier {
	return c.owner
}

// Run 启动飞书平台，并阻塞直到 ctx 被取消。
func (c *FeishuChannel) Run(ctx context.Context, handler Handler) error {
	c.mu.Lock()
	c.msgHandler = handler
	c.mu.Unlock()

	// 将飞书平台的消息处理回调适配为 channel.Handler
	platformHandler := func(p core.Platform, msg *core.Message) {
		c.mu.Lock()
		h := c.msgHandler
		c.mu.Unlock()
		if h == nil {
			slog.Warn("feishu: message received but no handler set")
			return
		}

		// 转换 core.Message 为 channel.Message
		chMsg := c.convertMessage(msg)
		if chMsg == nil {
			return
		}
		h(ctx, *chMsg)
	}

	if err := c.platform.Start(platformHandler); err != nil {
		return fmt.Errorf("feishu: start platform: %w", err)
	}

	// 等待 ctx 取消，然后停止平台
	<-ctx.Done()
	if err := c.platform.Stop(); err != nil {
		slog.Error("feishu: stop platform", "error", err)
	}
	return nil
}

// convertMessage 将飞书的 core.Message 转换为 channel.Message。
func (c *FeishuChannel) convertMessage(msg *core.Message) *Message {
	if msg == nil {
		return nil
	}

	var cmd *Command
	content := msg.Content
	// 检测命令（以 "/" 开头）
	if strings.HasPrefix(content, "/") {
		cmd = parseCommand(content)
		if cmd != nil {
			// 命令处理时，文本内容去掉命令前缀，只保留参数（如果需要）
			// 这里我们保持原样，让上层决定
		}
	}

	// 转换附件
	var attachments []transport.Attachment
	if len(msg.Images) > 0 {
		for _, img := range msg.Images {
			attachments = append(attachments, transport.Attachment{
				Filename: "", // 飞书图片没有文件名
				MimeType: img.MimeType,
				Data:     img.Data,
			})
		}
	}
	if len(msg.Files) > 0 {
		for _, f := range msg.Files {
			attachments = append(attachments, transport.Attachment{
				Filename: f.FileName,
				MimeType: f.MimeType,
				Data:     f.Data,
			})
		}
	}
	if msg.Audio != nil {
		attachments = append(attachments, transport.Attachment{
			Filename: "audio." + msg.Audio.Format,
			MimeType: msg.Audio.MimeType,
			Data:     msg.Audio.Data,
		})
	}

	// 构建 reply 适配器，用于回复当前消息
	reply := &feishuMessageReplier{
		platform: c.platform,
		replyCtx: msg.ReplyCtx,
	}

	return &Message{
		Text:        content,
		Attachments: attachments,
		Command:     cmd,
		Reply:       reply,
	}
}

// parseCommand 从文本中解析命令。
func parseCommand(text string) *Command {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return nil
	}
	cmdName := strings.TrimPrefix(parts[0], "/")
	if cmdName == "" {
		return nil
	}
	args := make(map[string]string)
	// 简单参数解析：支持 key=value 形式
	for _, p := range parts[1:] {
		if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
			args[kv[0]] = kv[1]
		} else {
			// 位置参数可以按顺序存放，这里简单用 index 作为 key
			args[fmt.Sprintf("arg%d", len(args)+1)] = p
		}
	}
	return &Command{
		Name: cmdName,
		Args: args,
	}
}

// newReplierForOwner 构建用于 owner 的 Replier。
func (c *FeishuChannel) newReplierForOwner() transport.Replier {
	// 优先使用 ownerChatID，否则使用 ownerUserID
	targetID := c.cfg.ownerChatID
	if targetID == "" {
		targetID = c.cfg.ownerUserID
	}
	if targetID == "" {
		slog.Warn("feishu: no owner chat_id or user_id configured, owner replier will be disabled")
	}
	return &feishuOwnerReplier{
		platform: c.platform,
		targetID: targetID,
	}
}

// feishuOwnerReplier 实现 transport.Replier，用于向 owner 发送消息。
type feishuOwnerReplier struct {
	platform *feishu.Platform
	targetID string
}

// 构造一个适合 owner 的 replyContext
func (r *feishuOwnerReplier) buildReplyCtx() any {
	if r.targetID == "" {
		return nil
	}
	// 利用 ReconstructReplyCtx 根据 sessionKey 构造 replyContext
	// sessionKey 格式：feishu:chatID:userID，这里 userID 用 targetID 代替
	sessionKey := fmt.Sprintf("feishu:%s:%s", r.targetID, r.targetID)
	ctx, err := r.platform.ReconstructReplyCtx(sessionKey)
	if err != nil {
		slog.Error("feishu: failed to reconstruct reply context for owner", "error", err)
		// 回退：尝试直接构造一个只包含 chatID 的 replyContext（可能不工作）
		// 由于 replyContext 是私有类型，无法直接构造，这里只能返回 nil，后续发送会失败
		return nil
	}
	return ctx
}

func (r *feishuOwnerReplier) SendText(ctx context.Context, text string) error {
	rc := r.buildReplyCtx()
	if rc == nil {
		return fmt.Errorf("feishu: cannot build reply context for owner")
	}
	return r.platform.Send(ctx, rc, text)
}

func (r *feishuOwnerReplier) SendFile(ctx context.Context, path, caption string) error {
	rc := r.buildReplyCtx()
	if rc == nil {
		return fmt.Errorf("feishu: cannot build reply context for owner")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("feishu: read file: %w", err)
	}
	// 发送文件
	if err := r.platform.SendFile(ctx, rc, core.FileAttachment{
		Data:     data,
		FileName: path,
		MimeType: detectMimeType(data),
	}); err != nil {
		return err
	}
	// 如果有 caption，单独发送文本
	if caption != "" {
		return r.platform.Send(ctx, rc, caption)
	}
	return nil
}

func (r *feishuOwnerReplier) SendTyping(ctx context.Context) error {
	// Owner replier 通常不需要 typing，直接返回 nil
	return nil
}

func (r *feishuOwnerReplier) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	rc := r.buildReplyCtx()
	if rc == nil {
		return fmt.Errorf("feishu: cannot build reply context for owner")
	}
	// 将询问转换为卡片
	card := buildAskCard(prompt, kind, options)
	if card == nil {
		// 降级为纯文本
		return r.platform.Send(ctx, rc, prompt)
	}
	// 发送卡片
	if p, ok := r.platform.(interface {
		SendCard(context.Context, any, *core.Card) error
	}); ok {
		return p.SendCard(ctx, rc, card)
	}
	// 如果不支持卡片，降级
	return r.platform.Send(ctx, rc, prompt)
}

func (r *feishuOwnerReplier) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	// 对于 owner，通常不需要流式输出，简化处理
	var finalText string
	for ev := range events {
		switch ev.Kind {
		case transport.Message:
			if data, ok := ev.Data.(transport.MessageData); ok {
				finalText = data.Text
			}
		case transport.Done:
			return finalText
		}
	}
	return finalText
}

// feishuMessageReplier 实现 transport.Replier，用于回复具体消息（带有 replyContext）。
type feishuMessageReplier struct {
	platform *feishu.Platform
	replyCtx any
}

func (r *feishuMessageReplier) SendText(ctx context.Context, text string) error {
	return r.platform.Reply(ctx, r.replyCtx, text)
}

func (r *feishuMessageReplier) SendFile(ctx context.Context, path, caption string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("feishu: read file: %w", err)
	}
	// 发送文件
	if err := r.platform.SendFile(ctx, r.replyCtx, core.FileAttachment{
		Data:     data,
		FileName: path,
		MimeType: detectMimeType(data),
	}); err != nil {
		return err
	}
	// 如果有 caption，单独发送文本
	if caption != "" {
		return r.platform.Reply(ctx, r.replyCtx, caption)
	}
	return nil
}

func (r *feishuMessageReplier) SendTyping(ctx context.Context) error {
	stop := r.platform.StartTyping(ctx, r.replyCtx)
	// 启动后立即停止？实际应该在 agent 处理期间保持 typing，这里我们只是触发一次
	// 更好的做法是在 ConsumeEvents 中控制，这里简单返回
	go func() {
		time.Sleep(5 * time.Second)
		stop()
	}()
	return nil
}

func (r *feishuMessageReplier) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	card := buildAskCard(prompt, kind, options)
	if card == nil {
		// 降级为纯文本
		return r.platform.Reply(ctx, r.replyCtx, prompt)
	}
	if p, ok := r.platform.(interface {
		SendCard(context.Context, any, *core.Card) error
	}); ok {
		return p.SendCard(ctx, r.replyCtx, card)
	}
	return r.platform.Reply(ctx, r.replyCtx, prompt)
}

// ConsumeEvents 处理来自 agent 的事件流，更新飞书消息。
func (r *feishuMessageReplier) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	// 使用预览模式：先发送一个占位消息，然后不断更新
	var previewHandle any
	var firstMessage bool = true
	var finalText string
	var lastUpdate time.Time
	updateInterval := 500 * time.Millisecond // 更新间隔

	// 收集需要更新的内容
	var currentText string
	var currentStatus *transport.AgentStatus
	var currentAsk *transport.AskData // 当前正在等待的 ask

	for ev := range events {
		select {
		case <-ctx.Done():
			return finalText
		default:
		}

		switch ev.Kind {
		case transport.LLMStream:
			if data, ok := ev.Data.(transport.LLMStreamData); ok {
				currentText += data.Text
			}
		case transport.LLMResponse, transport.Message:
			if data, ok := ev.Data.(transport.MessageData); ok {
				currentText += data.Text
			}
		case transport.Progress:
			if data, ok := ev.Data.(transport.ProgressData); ok {
				// 构建状态更新
				status := &transport.AgentStatus{
					Phase:   transport.PhaseWorking,
					Detail:  data.Status,
					Plan:    data.Tasks,
					Percent: data.Percent,
				}
				currentStatus = status
			}
		case transport.ToolStart:
			if data, ok := ev.Data.(transport.ToolStartData); ok {
				currentStatus = &transport.AgentStatus{
					Phase:  transport.PhaseTool,
					Tool:   data.Name,
					Detail: data.Label,
				}
			}
		case transport.ToolEnd:
			// 工具结束，可能恢复到 working 状态
			currentStatus = &transport.AgentStatus{
				Phase: transport.PhaseWorking,
			}
		case transport.Ask:
			if data, ok := ev.Data.(transport.AskData); ok {
				// 暂停事件处理，等待用户输入
				currentAsk = &data
				// 发送询问卡片
				card := buildAskCard(data.Prompt, data.Kind, data.Options)
				if card != nil {
					// 发送卡片并等待回复
					// 注意：这里需要将 replyInbox 与卡片按钮关联
					// 飞书卡片通过回调返回用户选择，我们需要将选择写入 replyInbox
					// 由于飞书卡片回调是异步的，我们无法在这里同步等待
					// 因此，我们需要一个全局的卡片回调处理器，将结果写入 replyInbox
					// 简化起见，这里假设 ask 已经被上层处理，我们只需发送卡片
					if p, ok := r.platform.(interface {
						SendCard(context.Context, any, *core.Card) error
					}); ok {
						_ = p.SendCard(ctx, r.replyCtx, card)
					} else {
						_ = r.platform.Reply(ctx, r.replyCtx, data.Prompt)
					}
				} else {
					_ = r.platform.Reply(ctx, r.replyCtx, data.Prompt)
				}
				// 注意：我们不能等待 replyInbox，因为这里阻塞会导致事件循环停止
				// 实际应用中，应该由上层在外部等待 replyInbox
			}
		case transport.Done:
			// 最终更新
			if previewHandle != nil {
				_ = r.updatePreview(ctx, previewHandle, finalText, currentStatus)
			}
			return finalText
		}

		// 更新预览消息
		if previewHandle == nil && (currentText != "" || currentStatus != nil) {
			// 首次发送预览
			content := buildPreviewContent(currentText, currentStatus)
			handle, err := r.platform.SendPreviewStart(ctx, r.replyCtx, content)
			if err != nil {
				slog.Error("feishu: send preview start", "error", err)
				// 降级：直接发送普通消息
				_ = r.platform.Reply(ctx, r.replyCtx, currentText)
			} else {
				previewHandle = handle
				firstMessage = false
				lastUpdate = time.Now()
			}
		} else if previewHandle != nil && (currentText != "" || currentStatus != nil) {
			// 限流更新
			if time.Since(lastUpdate) >= updateInterval {
				content := buildPreviewContent(currentText, currentStatus)
				if err := r.updatePreview(ctx, previewHandle, content, currentStatus); err != nil {
					slog.Error("feishu: update preview", "error", err)
				}
				lastUpdate = time.Now()
			}
		}
	}
	return finalText
}

func (r *feishuMessageReplier) updatePreview(ctx context.Context, handle any, text string, status *transport.AgentStatus) error {
	content := buildPreviewContent(text, status)
	return r.platform.UpdateMessage(ctx, handle, content)
}

// buildPreviewContent 构建预览消息的文本（可包含状态）
func buildPreviewContent(text string, status *transport.AgentStatus) string {
	var sb strings.Builder
	if status != nil {
		switch status.Phase {
		case transport.PhaseThinking:
			sb.WriteString("🤔 思考中...")
		case transport.PhasePlanning:
			sb.WriteString("📋 规划中...")
		case transport.PhaseTool:
			sb.WriteString(fmt.Sprintf("🔧 使用工具: %s", status.Detail))
		case transport.PhaseWorking:
			if status.Detail != "" {
				sb.WriteString(fmt.Sprintf("⚙️ %s", status.Detail))
			} else {
				sb.WriteString("⚙️ 工作中...")
			}
		}
		if status.Percent != nil {
			sb.WriteString(fmt.Sprintf(" %d%%", *status.Percent))
		}
		sb.WriteString("\n\n")
	}
	if text != "" {
		sb.WriteString(text)
	} else {
		sb.WriteString("正在处理...")
	}
	return sb.String()
}

// buildAskCard 构建询问卡片
func buildAskCard(prompt string, kind transport.AskKind, options []transport.AskOption) *core.Card {
	card := core.NewCard().Title("需要您的输入", "blue").Markdown(prompt)
	switch kind {
	case transport.AskConfirm:
		card.Actions([]core.CardButton{
			{Text: "✅ 确认", Value: "confirm", Type: "primary"},
			{Text: "❌ 取消", Value: "cancel", Type: "default"},
		})
	case transport.AskChoice:
		buttons := make([]core.CardButton, 0, len(options))
		for _, opt := range options {
			buttons = append(buttons, core.CardButton{
				Text:  opt.Label,
				Value: opt.Value,
				Type:  "default",
			})
		}
		card.Actions(buttons)
	case transport.AskFreeform:
		// 飞书卡片没有原生文本输入框，但可以用 post 或 markdown 中的可编辑区域？
		// 暂时降级为文本消息
		return nil
	}
	return card
}

// 辅助函数：检测 MIME 类型
func detectMimeType(data []byte) string {
	if len(data) >= 8 {
		if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
			return "image/png"
		}
		if data[0] == 0xFF && data[1] == 0xD8 {
			return "image/jpeg"
		}
		if string(data[:4]) == "GIF8" {
			return "image/gif"
		}
		if string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
			return "image/webp"
		}
	}
	return "application/octet-stream"
}
