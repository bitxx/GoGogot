package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Core send helpers
// ---------------------------------------------------------------------------

// sendMsg sends a single message and returns its message_id.
func (c *Channel) sendMsg(ctx context.Context, chatID, msgType, content string) string {
	resp, err := c.api.Im.Message.Create(ctx,
		larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(larkim.ReceiveIdTypeChatId).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(chatID).
				MsgType(msgType).
				Content(content).
				Build()).
			Build())
	if err != nil || !resp.Success() {
		log.Error().Err(err).
			Int("code", resp.Code).Str("msg", resp.Msg).
			Msg("feishu: sendMsg failed")
		return ""
	}
	return strVal(resp.Data.MessageId)
}

// sendText sends a plain-text message and returns its message_id.
func (c *Channel) sendText(ctx context.Context, chatID, text string) string {
	b, _ := json.Marshal(map[string]string{"text": text})
	return c.sendMsg(ctx, chatID, larkim.MsgTypeText, string(b))
}

// sendCard sends an interactive card and returns its message_id.
func (c *Channel) sendCard(ctx context.Context, chatID, cardJSON string) string {
	return c.sendMsg(ctx, chatID, larkim.MsgTypeInteractive, cardJSON)
}

// sendAndGetID mirrors the Telegram helper used by status.go.
func (c *Channel) sendAndGetID(ctx context.Context, chatID, text string) string {
	return c.sendText(ctx, chatID, text)
}

// sendLong splits text into chunks and sends each as a plain-text message.
func (c *Channel) sendLong(ctx context.Context, chatID, text string) {
	for _, chunk := range splitText(text, maxMessageLen) {
		c.sendText(ctx, chatID, chunk)
	}
}

// sendCardLong sends text as a markdown card, splitting if needed.
func (c *Channel) sendCardLong(ctx context.Context, chatID, text string) {
	for _, chunk := range splitText(text, maxCardLen) {
		if c.sendCard(ctx, chatID, markdownCard(chunk)) == "" {
			c.sendText(ctx, chatID, chunk)
		}
	}
}

// patchMsg edits an existing message in-place (card content only).
func (c *Channel) patchMsg(ctx context.Context, msgID, content string) error {
	resp, err := c.api.Im.Message.Patch(ctx,
		larkim.NewPatchMessageReqBuilder().
			MessageId(msgID).
			Body(larkim.NewPatchMessageReqBodyBuilder().
				Content(content).
				Build()).
			Build())
	if err != nil || !resp.Success() {
		return fmt.Errorf("feishu patchMsg code=%d msg=%s: %w", resp.Code, resp.Msg, firstErr(err))
	}
	return nil
}

// deleteMsg deletes a message by its message_id.
func (c *Channel) deleteMsg(ctx context.Context, msgID string) error {
	resp, err := c.api.Im.Message.Delete(ctx,
		larkim.NewDeleteMessageReqBuilder().
			MessageId(msgID).
			Build())
	if err != nil || !resp.Success() {
		return fmt.Errorf("feishu deleteMsg code=%d msg=%s: %w", resp.Code, resp.Msg, firstErr(err))
	}
	return nil
}

// ---------------------------------------------------------------------------
// File / image send helpers
// ---------------------------------------------------------------------------

func (c *Channel) sendImage(ctx context.Context, chatID string, data []byte, caption string) error {
	uploadResp, err := c.api.Im.Image.Create(ctx,
		larkim.NewCreateImageReqBuilder().
			Body(larkim.NewCreateImageReqBodyBuilder().
				ImageType("message").
				Image(bytes.NewReader(data)).
				Build()).
			Build())
	if err != nil || !uploadResp.Success() {
		return fmt.Errorf("feishu upload image code=%d: %w", uploadResp.Code, firstErr(err))
	}
	imageKey := strVal(uploadResp.Data.ImageKey)
	b, _ := json.Marshal(map[string]string{"image_key": imageKey})
	c.sendMsg(ctx, chatID, larkim.MsgTypeImage, string(b))
	if caption != "" {
		c.sendLong(ctx, chatID, caption)
	}
	return nil
}

func (c *Channel) sendFile(ctx context.Context, chatID string, data []byte, filename, caption string) error {
	uploadResp, err := c.api.Im.File.Create(ctx,
		larkim.NewCreateFileReqBuilder().
			Body(larkim.NewCreateFileReqBodyBuilder().
				FileType(fileType(filename)).
				FileName(filename).
				File(bytes.NewReader(data)).
				Build()).
			Build())
	if err != nil || !uploadResp.Success() {
		return fmt.Errorf("feishu upload file code=%d: %w", uploadResp.Code, firstErr(err))
	}
	fileKey := strVal(uploadResp.Data.FileKey)
	b, _ := json.Marshal(map[string]string{"file_key": fileKey})
	c.sendMsg(ctx, chatID, larkim.MsgTypeFile, string(b))
	if caption != "" {
		c.sendLong(ctx, chatID, caption)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Card builders
// ---------------------------------------------------------------------------

func markdownCard(text string) string {
	card := map[string]any{
		"schema": "2.0",
		"config": map[string]any{"wide_screen_mode": false},
		"body": map[string]any{
			"elements": []any{
				map[string]any{"tag": "markdown", "content": text},
			},
		},
	}
	b, _ := json.Marshal(card)
	return string(b)
}

func headerCard(headerText, color, bodyText string) string {
	elements := []any{}
	if bodyText != "" {
		elements = append(elements, map[string]any{"tag": "markdown", "content": bodyText})
	}
	card := map[string]any{
		"schema": "2.0",
		"config": map[string]any{"wide_screen_mode": false},
		"header": map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": headerText},
			"template": color,
		},
		"body": map[string]any{"elements": elements},
	}
	b, _ := json.Marshal(card)
	return string(b)
}

type buttonOpt struct {
	Label, Value string
}

func buttonCard(prompt string, buttons []buttonOpt) string {
	actions := make([]any, len(buttons))
	for i, btn := range buttons {
		actions[i] = map[string]any{
			"tag":   "button",
			"text":  map[string]any{"tag": "plain_text", "content": btn.Label},
			"value": map[string]string{"value": btn.Value},
		}
	}
	card := map[string]any{
		"schema": "2.0",
		"config": map[string]any{"wide_screen_mode": false},
		"header": map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": "❓ " + prompt},
			"template": "blue",
		},
		"body": map[string]any{
			"elements": []any{
				map[string]any{"tag": "action", "actions": actions},
			},
		},
	}
	b, _ := json.Marshal(card)
	return string(b)
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

func splitText(text string, limit int) []string {
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}
	var chunks []string
	for len(runes) > 0 {
		cut := limit
		if cut > len(runes) {
			cut = len(runes)
		}
		sub := string(runes[:cut])
		if idx := strings.LastIndex(sub, "\n"); idx > 0 {
			cut = len([]rune(sub[:idx+1]))
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	return chunks
}

func isTextMIME(mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/json", "application/xml", "application/javascript",
		"application/x-yaml", "application/toml", "application/x-sh",
		"application/csv", "application/sql":
		return true
	}
	return false
}

func mimeFromFilename(name string) string {
	lower := strings.ToLower(name)
	switch {
	case hasSuffix(lower, ".jpg", ".jpeg"):
		return "image/jpeg"
	case hasSuffix(lower, ".png"):
		return "image/png"
	case hasSuffix(lower, ".webp"):
		return "image/webp"
	case hasSuffix(lower, ".gif"):
		return "image/gif"
	case hasSuffix(lower, ".pdf"):
		return "application/pdf"
	case hasSuffix(lower, ".json"):
		return "application/json"
	case hasSuffix(lower, ".yaml", ".yml"):
		return "application/x-yaml"
	case hasSuffix(lower, ".mp4", ".mov"):
		return "video/mp4"
	case hasSuffix(lower, ".mp3", ".m4a"):
		return "audio/mpeg"
	case hasSuffix(lower, ".ogg", ".opus"):
		return "audio/ogg"
	default:
		return "text/plain"
	}
}

func fileType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case hasSuffix(lower, ".mp4", ".mov", ".avi", ".mkv"):
		return "mp4"
	case hasSuffix(lower, ".ogg", ".opus"):
		return "opus"
	case hasSuffix(lower, ".pdf"):
		return "pdf"
	case hasSuffix(lower, ".doc", ".docx"):
		return "doc"
	case hasSuffix(lower, ".xls", ".xlsx"):
		return "xls"
	case hasSuffix(lower, ".ppt", ".pptx"):
		return "ppt"
	case hasSuffix(lower, ".zip"):
		return "zip"
	default:
		return "stream"
	}
}

func hasSuffix(s string, suffixes ...string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

func firstErr(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("sdk error")
}
