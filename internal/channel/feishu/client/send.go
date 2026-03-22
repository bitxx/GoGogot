package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"strings"
)

// SendMessage 发送文本消息到指定会话（群聊或单聊）
// chatID: 会话ID（可以是 open_id, user_id, chat_id）
// text: 文本内容
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	// 构造文本消息内容
	content := map[string]string{"text": text}
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal text: %w", err)
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id"). // 统一使用 chat_id 作为接收者类型
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType("text").
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := c.client.Im.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// ReplyMessage 回复指定消息
// messageID: 要回复的消息ID
// text: 回复内容
func (c *Client) ReplyMessage(ctx context.Context, messageID, text string) error {
	content := map[string]string{"text": text}
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("marshal reply: %w", err)
	}

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType("text").
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := c.client.Im.Message.Reply(ctx, req)
	if err != nil {
		return fmt.Errorf("reply message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("reply message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// SendImage 发送图片到指定会话（支持回复）
// chatID: 会话ID
// messageID: 可选，若提供则作为回复发送
// imageData: 图片二进制数据
// imageType: 图片类型（如 "png", "jpeg"），仅用于 MIME 检测
func (c *Client) SendImage(ctx context.Context, chatID, messageID string, imageData []byte, imageType string) error {
	// 1. 上传图片
	uploadResp, err := c.client.Im.Image.Create(ctx,
		larkim.NewCreateImageReqBuilder().
			Body(larkim.NewCreateImageReqBodyBuilder().
				ImageType("message").
				Image(bytes.NewReader(imageData)).
				Build()).
			Build())
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}
	if !uploadResp.Success() {
		return fmt.Errorf("upload image failed: code=%d msg=%s", uploadResp.Code, uploadResp.Msg)
	}
	if uploadResp.Data == nil || uploadResp.Data.ImageKey == nil {
		return fmt.Errorf("upload image: missing image_key")
	}
	imageKey := *uploadResp.Data.ImageKey

	// 2. 构建图片消息内容
	imageContent, err := (&larkim.MessageImage{ImageKey: imageKey}).String()
	if err != nil {
		return fmt.Errorf("build image message: %w", err)
	}

	// 3. 发送
	return c.sendMediaMessage(ctx, chatID, messageID, "image", imageContent)
}

// SendFile 发送文件到指定会话
// chatID: 会话ID
// messageID: 可选，若提供则作为回复发送
// fileData: 文件二进制数据
// fileName: 文件名（用于推断类型）
func (c *Client) SendFile(ctx context.Context, chatID, messageID string, fileData []byte, fileName string) error {
	// 1. 检测文件类型
	fileType := detectFeishuFileType(fileName)

	// 2. 上传文件
	uploadResp, err := c.client.Im.File.Create(ctx,
		larkim.NewCreateFileReqBuilder().
			Body(larkim.NewCreateFileReqBodyBuilder().
				FileType(fileType).
				FileName(fileName).
				File(bytes.NewReader(fileData)).
				Build()).
			Build())
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}
	if !uploadResp.Success() {
		return fmt.Errorf("upload file failed: code=%d msg=%s", uploadResp.Code, uploadResp.Msg)
	}
	if uploadResp.Data == nil || uploadResp.Data.FileKey == nil {
		return fmt.Errorf("upload file: missing file_key")
	}
	fileKey := *uploadResp.Data.FileKey

	// 3. 构建文件消息内容
	fileContent, err := (&larkim.MessageFile{FileKey: fileKey}).String()
	if err != nil {
		return fmt.Errorf("build file message: %w", err)
	}

	// 4. 发送
	return c.sendMediaMessage(ctx, chatID, messageID, "file", fileContent)
}

// SendAudio 发送音频到指定会话
// chatID: 会话ID
// messageID: 可选，若提供则作为回复发送
// audioData: 音频二进制数据
// format: 音频格式（如 "opus", "mp3"），若非 opus 会尝试转换
func (c *Client) SendAudio(ctx context.Context, chatID, messageID string, audioData []byte, format string) error {
	// 如果不是 opus 格式，尝试转换（需要 ffmpeg）
	if format != "opus" {
		converted, err := convertToOpus(ctx, audioData, format)
		if err != nil {
			return fmt.Errorf("convert audio to opus: %w", err)
		}
		audioData = converted
		format = "opus"
	}

	// 1. 上传音频（文件类型 opus）
	uploadResp, err := c.client.Im.File.Create(ctx,
		larkim.NewCreateFileReqBuilder().
			Body(larkim.NewCreateFileReqBodyBuilder().
				FileType(larkim.FileTypeOpus).
				FileName("audio.opus").
				File(bytes.NewReader(audioData)).
				Build()).
			Build())
	if err != nil {
		return fmt.Errorf("upload audio: %w", err)
	}
	if !uploadResp.Success() {
		return fmt.Errorf("upload audio failed: code=%d msg=%s", uploadResp.Code, uploadResp.Msg)
	}
	if uploadResp.Data == nil || uploadResp.Data.FileKey == nil {
		return fmt.Errorf("upload audio: missing file_key")
	}
	fileKey := *uploadResp.Data.FileKey

	// 2. 构建音频消息内容
	audioContent, err := (&larkim.MessageAudio{FileKey: fileKey}).String()
	if err != nil {
		return fmt.Errorf("build audio message: %w", err)
	}

	// 3. 发送
	return c.sendMediaMessage(ctx, chatID, messageID, "audio", audioContent)
}

// SendTyping 发送“正在输入”状态（通过添加表情反应，需要消息ID）
// messageID: 要添加反应的消息ID
// 返回一个停止函数，调用后移除反应
func (c *Client) SendTyping(ctx context.Context, messageID string) (stop func(), err error) {
	// 长代码中使用的表情是 "OnIt"，可配置，这里固定使用
	emojiType := "OnIt"
	resp, err := c.client.Im.MessageReaction.Create(ctx,
		larkim.NewCreateMessageReactionReqBuilder().
			MessageId(messageID).
			Body(larkim.NewCreateMessageReactionReqBodyBuilder().
				ReactionType(&larkim.Emoji{EmojiType: &emojiType}).
				Build()).
			Build())
	if err != nil {
		return nil, fmt.Errorf("add reaction: %w", err)
	}
	if !resp.Success() {
		return nil, fmt.Errorf("add reaction failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	reactionID := ""
	if resp.Data != nil && resp.Data.ReactionId != nil {
		reactionID = *resp.Data.ReactionId
	}

	// 返回移除反应的函数
	stop = func() {
		if reactionID == "" {
			return
		}
		delResp, err := c.client.Im.MessageReaction.Delete(ctx,
			larkim.NewDeleteMessageReactionReqBuilder().
				MessageId(messageID).
				ReactionId(reactionID).
				Build())
		if err != nil {
			// 忽略删除失败
			return
		}
		_ = delResp
	}
	return stop, nil
}

// sendMediaMessage 内部辅助函数：发送媒体消息（图片、文件、音频）
func (c *Client) sendMediaMessage(ctx context.Context, chatID, messageID, msgType, content string) error {
	if messageID != "" {
		// 作为回复发送
		req := larkim.NewReplyMessageReqBuilder().
			MessageId(messageID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType(msgType).
				Content(content).
				Build()).
			Build()
		resp, err := c.client.Im.Message.Reply(ctx, req)
		if err != nil {
			return fmt.Errorf("send media as reply: %w", err)
		}
		if !resp.Success() {
			return fmt.Errorf("send media as reply failed: code=%d msg=%s", resp.Code, resp.Msg)
		}
		return nil
	}

	// 作为新消息发送
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()
	resp, err := c.client.Im.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("send media: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send media failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

// detectFeishuFileType 根据文件名推断飞书文件类型（从长代码中抽离）
func detectFeishuFileType(fileName string) string {
	name := strings.ToLower(fileName)
	switch {
	case strings.HasSuffix(name, ".pdf"):
		return larkim.FileTypePdf
	case strings.HasSuffix(name, ".doc") || strings.HasSuffix(name, ".docx"):
		return larkim.FileTypeDoc
	case strings.HasSuffix(name, ".xls") || strings.HasSuffix(name, ".xlsx") || strings.HasSuffix(name, ".csv"):
		return larkim.FileTypeXls
	case strings.HasSuffix(name, ".ppt") || strings.HasSuffix(name, ".pptx"):
		return larkim.FileTypePpt
	case strings.HasSuffix(name, ".mp4"):
		return larkim.FileTypeMp4
	case strings.HasSuffix(name, ".opus"):
		return larkim.FileTypeOpus
	default:
		return larkim.FileTypeStream
	}
}

// convertToOpus 将非 opus 音频转换为 opus（简化实现，实际可使用 ffmpeg 等工具）
// 长代码中使用了 core.ConvertAudioToOpus，这里占位，实际项目需集成
func convertToOpus(ctx context.Context, audioData []byte, format string) ([]byte, error) {
	// 这是一个占位函数，实际应调用 ffmpeg 或其他转换库
	// 由于长代码中依赖 core 包，此处仅作为结构占位
	return nil, fmt.Errorf("audio conversion not implemented; please provide opus audio")
}
