package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/rs/zerolog/log"

	"gogogot/internal/channel"
	"gogogot/internal/core/transport"
)

// convertAndDispatch parses the incoming Feishu message, downloads any
// attachments, and calls c.handler — mirroring Telegram's convertAndDispatch.
func (c *Channel) convertAndDispatch(ctx context.Context, chatID string, msg *larkim.EventMessage) {
	reply := c.newReplier(chatID)
	msgType := strVal(msg.MessageType)
	content := strVal(msg.Content)
	messageID := strVal(msg.MessageId)

	var textParts []string
	var attachments []transport.Attachment

	switch msgType {
	case "text":
		if t := parseTextContent(content); t != "" {
			textParts = append(textParts, t)
		}

	case "image":
		var ic struct {
			ImageKey string `json:"image_key"`
		}
		if err := json.Unmarshal([]byte(content), &ic); err == nil && ic.ImageKey != "" {
			atts, err := c.processImage(ctx, messageID, ic.ImageKey)
			if err != nil {
				log.Error().Err(err).Msg("feishu: process image failed")
			} else {
				attachments = append(attachments, atts...)
			}
		}

	case "file":
		var fc struct {
			FileKey  string `json:"file_key"`
			FileName string `json:"file_name"`
			FileSize int64  `json:"file_size"`
			MimeType string `json:"mime_type"`
		}
		if err := json.Unmarshal([]byte(content), &fc); err == nil && fc.FileKey != "" {
			atts, err := c.processFile(ctx, messageID, fc.FileKey, fc.FileName, fc.MimeType, fc.FileSize)
			if err != nil {
				log.Error().Err(err).Msg("feishu: process file failed")
			} else {
				attachments = append(attachments, atts...)
			}
		}

	case "audio":
		var ac struct {
			FileKey  string `json:"file_key"`
			Duration int    `json:"duration"`
		}
		if err := json.Unmarshal([]byte(content), &ac); err == nil && ac.FileKey != "" {
			atts, err := c.downloadChecked(ctx, messageID, ac.FileKey, "audio", maxGenericFileSize, "voice.ogg", "audio/ogg")
			if err != nil {
				log.Error().Err(err).Msg("feishu: process audio failed")
			} else {
				attachments = append(attachments, atts...)
			}
		}

	case "sticker":
		var sc struct {
			FileKey string `json:"file_key"`
		}
		if err := json.Unmarshal([]byte(content), &sc); err == nil && sc.FileKey != "" {
			// Only download static stickers (no animated check available via API)
			atts, err := c.downloadChecked(ctx, messageID, sc.FileKey, "image", maxImageFileSize, "sticker.png", "image/png")
			if err != nil {
				log.Error().Err(err).Msg("feishu: process sticker failed")
			} else {
				attachments = append(attachments, atts...)
			}
		}

	case "post":
		if t := extractPostText(content); t != "" {
			textParts = append(textParts, t)
		}

	default:
		log.Trace().Str("type", msgType).Msg("feishu: unsupported message type, ignoring")
		return
	}

	// Promote text-MIME file attachments into the message body (same as Telegram).
	var fileTexts []string
	for _, att := range attachments {
		if !strings.HasPrefix(att.MimeType, "image/") && isTextMIME(att.MimeType) {
			fileTexts = append(fileTexts, fmt.Sprintf("[File: %s]\n```\n%s\n```", att.Filename, string(att.Data)))
		}
	}
	if len(fileTexts) > 0 {
		filesStr := strings.Join(fileTexts, "\n\n")
		textParts = append([]string{filesStr}, textParts...)
	}

	text := strings.Join(textParts, "\n\n")

	if text == "" && len(attachments) == 0 {
		return
	}
	if text == "" && len(attachments) > 0 {
		text = "What's in these files?"
	}

	if strings.HasPrefix(text, "/") {
		cmdName := strings.Fields(text)[0]
		log.Info().Str("cmd", cmdName).Msg("feishu: command received")
		c.handleCommand(ctx, chatID, reply, cmdName)
		return
	}

	c.handler(ctx, channel.Message{
		Text:        text,
		Attachments: attachments,
		Reply:       reply,
	})
}

// ---------------------------------------------------------------------------
// Per-type media processors — mirrors Telegram's media.go
// ---------------------------------------------------------------------------

// processImage downloads an image attachment with size check.
func (c *Channel) processImage(ctx context.Context, messageID, imageKey string) ([]transport.Attachment, error) {
	return c.downloadChecked(ctx, messageID, imageKey, "image", maxImageFileSize, "photo.jpg", "image/jpeg")
}

// processFile handles file attachments including zip/tar.gz expansion.
func (c *Channel) processFile(ctx context.Context, messageID, fileKey, filename, mime string, fileSize int64) ([]transport.Attachment, error) {
	if filename == "" {
		filename = fileKey
	}
	if mime == "" {
		mime = mimeFromFilename(filename)
	}

	// Expand zip archives.
	if isArchiveZip(mime, filename) {
		if fileSize > maxGenericFileSize {
			return nil, fmt.Errorf("zip file too large (%d bytes)", fileSize)
		}
		data, err := c.downloadRaw(ctx, messageID, fileKey, "file")
		if err != nil {
			return nil, err
		}
		return extractZipFiles(data)
	}

	// Expand tar.gz archives.
	if isArchiveTarGz(mime, filename) {
		if fileSize > maxGenericFileSize {
			return nil, fmt.Errorf("tar.gz file too large (%d bytes)", fileSize)
		}
		data, err := c.downloadRaw(ctx, messageID, fileKey, "file")
		if err != nil {
			return nil, err
		}
		return extractTarGzFiles(data)
	}

	// Image file sent as a "file" message.
	if isImageMIME(mime) {
		return c.downloadChecked(ctx, messageID, fileKey, "file", maxImageFileSize, filename, mime)
	}

	// Text / code files.
	if isTextMIME(mime) || mime == "application/octet-stream" {
		return c.downloadChecked(ctx, messageID, fileKey, "file", maxTextFileSize, filename, "text/plain")
	}

	// Generic binary.
	return c.downloadChecked(ctx, messageID, fileKey, "file", maxGenericFileSize, filename, mime)
}

// ---------------------------------------------------------------------------
// Download helpers
// ---------------------------------------------------------------------------

// downloadChecked downloads a resource and enforces a size limit.
// Since Feishu's API does not expose file size before download for all types,
// we check after reading — and discard if over limit.
func (c *Channel) downloadChecked(ctx context.Context, messageID, fileKey, resourceType string, maxSize int64, filename, mime string) ([]transport.Attachment, error) {
	data, err := c.downloadRaw(ctx, messageID, fileKey, resourceType)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("file too large (%d bytes)", len(data))
	}
	return []transport.Attachment{{Filename: filename, MimeType: mime, Data: data}}, nil
}

// downloadRaw calls the SDK MessageResource.Get and returns raw bytes.
func (c *Channel) downloadRaw(ctx context.Context, messageID, fileKey, resourceType string) ([]byte, error) {
	resp, err := c.api.Im.MessageResource.Get(ctx,
		larkim.NewGetMessageResourceReqBuilder().
			MessageId(messageID).
			FileKey(fileKey).
			Type(resourceType).
			Build())
	if err != nil || !resp.Success() {
		return nil, fmt.Errorf("feishu downloadRaw code=%d: %w", resp.Code, firstErr(err))
	}
	defer func() {
		if rc, ok := resp.File.(interface{ Close() error }); ok {
			_ = rc.Close()
		}
	}()
	return io.ReadAll(resp.File)
}

// ---------------------------------------------------------------------------
// Content parsers
// ---------------------------------------------------------------------------

func parseTextContent(content string) string {
	var tc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &tc); err != nil {
		return ""
	}
	text := tc.Text
	for {
		start := strings.Index(text, "<at")
		if start < 0 {
			break
		}
		gt := strings.Index(text[start:], ">")
		if gt < 0 {
			break
		}
		closeTag := strings.Index(text[start+gt+1:], "</at>")
		if closeTag < 0 {
			break
		}
		name := text[start+gt+1 : start+gt+1+closeTag]
		text = text[:start] + "@" + name + text[start+gt+1+closeTag+5:]
	}
	return strings.TrimSpace(text)
}

type postBody struct {
	ZhCN *postLang `json:"zh_cn,omitempty"`
	EnUS *postLang `json:"en_us,omitempty"`
}

type postLang struct {
	Title   string       `json:"title"`
	Content [][]postElem `json:"content"`
}

type postElem struct {
	Tag      string `json:"tag"`
	Text     string `json:"text,omitempty"`
	Href     string `json:"href,omitempty"`
	UserName string `json:"user_name,omitempty"`
}

func extractPostText(content string) string {
	var wrapper struct {
		Content postBody `json:"content"`
	}
	if err := json.Unmarshal([]byte(content), &wrapper); err != nil {
		return ""
	}
	lang := wrapper.Content.ZhCN
	if lang == nil {
		lang = wrapper.Content.EnUS
	}
	if lang == nil {
		return ""
	}
	var sb strings.Builder
	if lang.Title != "" {
		sb.WriteString(lang.Title)
		sb.WriteString("\n\n")
	}
	for _, row := range lang.Content {
		for _, el := range row {
			switch el.Tag {
			case "text":
				sb.WriteString(el.Text)
			case "a":
				sb.WriteString(el.Text)
				if el.Href != "" {
					sb.WriteString(" (" + el.Href + ")")
				}
			case "at":
				sb.WriteString("@" + el.UserName)
			}
		}
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}
