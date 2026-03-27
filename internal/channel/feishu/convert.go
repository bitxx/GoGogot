package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"gogogot/internal/channel"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/rs/zerolog/log"

	"gogogot/internal/core/transport"
)

// convertAndDispatch parses the Feishu message and calls c.handler.
// It mirrors Telegram's convertAndDispatch as closely as possible.
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
			atts, err := c.downloadResource(ctx, messageID, ic.ImageKey, "image")
			if err != nil {
				log.Error().Err(err).Msg("feishu: download image failed")
			} else {
				attachments = append(attachments, atts...)
			}
		}

	case "file":
		var fc struct {
			FileKey  string `json:"file_key"`
			FileName string `json:"file_name"`
		}
		if err := json.Unmarshal([]byte(content), &fc); err == nil && fc.FileKey != "" {
			atts, err := c.downloadResource(ctx, messageID, fc.FileKey, "file")
			if err != nil {
				log.Error().Err(err).Msg("feishu: download file failed")
			} else {
				// Override filename from the message payload.
				if len(atts) > 0 && fc.FileName != "" {
					atts[0].Filename = fc.FileName
					atts[0].MimeType = mimeFromFilename(fc.FileName)
				}
				attachments = append(attachments, atts...)
			}
		}

	case "audio":
		var ac struct {
			FileKey string `json:"file_key"`
		}
		if err := json.Unmarshal([]byte(content), &ac); err == nil && ac.FileKey != "" {
			atts, err := c.downloadResource(ctx, messageID, ac.FileKey, "audio")
			if err != nil {
				log.Error().Err(err).Msg("feishu: download audio failed")
			} else {
				attachments = append(attachments, atts...)
			}
		}

	case "post":
		// Rich-text (飞书富文本) → plain text
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

	// Command routing.
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
// Content parsers
// ---------------------------------------------------------------------------

func parseTextContent(content string) string {
	var tc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &tc); err != nil {
		return ""
	}
	// Strip Feishu <at uid="...">Name</at> tags, keep @Name.
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

// Feishu "post" (富文本) structures.
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
