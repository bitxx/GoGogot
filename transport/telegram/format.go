package telegram

import (
	"strings"

	"gogogot/markdown"
)

// FormattedChunk holds both the HTML and plain-text version of a chunk,
// so we can fall back to plain text if Telegram rejects the HTML.
type FormattedChunk struct {
	HTML string
	Text string
}

var telegramMarkers = markdown.RenderOptions{
	StyleMarkers: map[markdown.Style]markdown.StyleMarker{
		markdown.Bold:          {Open: "<b>", Close: "</b>"},
		markdown.Italic:        {Open: "<i>", Close: "</i>"},
		markdown.Strikethrough: {Open: "<s>", Close: "</s>"},
		markdown.Code:          {Open: "<code>", Close: "</code>"},
		markdown.CodeBlock:     {Open: "<pre><code>", Close: "</code></pre>"},
		markdown.Blockquote:    {Open: "<blockquote>", Close: "</blockquote>"},
	},
	EscapeText: escapeHTML,
	BuildLink:  buildTelegramLink,
}

// FormatHTML converts standard markdown into Telegram-compatible HTML.
func FormatHTML(text string) string {
	ir := markdown.Parse(text)
	return markdown.Render(ir, telegramMarkers)
}

// FormatHTMLChunks converts markdown into Telegram HTML chunks, each within
// the byte limit. Styles are preserved across chunk boundaries.
func FormatHTMLChunks(text string, limit int) []FormattedChunk {
	ir := markdown.Parse(text)
	chunks := markdown.ChunkIR(ir, limit)
	if len(chunks) == 0 && text != "" {
		return []FormattedChunk{{HTML: escapeHTML(text), Text: text}}
	}
	result := make([]FormattedChunk, 0, len(chunks))
	for _, chunk := range chunks {
		result = append(result, FormattedChunk{
			HTML: markdown.Render(chunk, telegramMarkers),
			Text: chunk.Text,
		})
	}
	return result
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escapeHTMLAttr(s string) string {
	return strings.ReplaceAll(escapeHTML(s), `"`, "&quot;")
}

// File extensions that overlap with TLDs. When these appear as bare filenames
// (e.g. README.md), Telegram's linkify turns them into domain previews.
// We suppress those auto-links.
var fileExtTLDs = map[string]bool{
	"md": true, "go": true, "py": true, "pl": true, "sh": true,
	"am": true, "at": true, "be": true, "cc": true,
}

func isAutoLinkedFileRef(href, label string) bool {
	stripped := strings.TrimPrefix(strings.TrimPrefix(href, "https://"), "http://")
	if stripped != label {
		return false
	}
	dot := strings.LastIndex(label, ".")
	if dot < 1 {
		return false
	}
	ext := strings.ToLower(label[dot+1:])
	return fileExtTLDs[ext]
}

func buildTelegramLink(link markdown.LinkSpan, text string) *markdown.RenderedLink {
	href := strings.TrimSpace(link.Href)
	if href == "" || link.Start == link.End {
		return nil
	}
	label := text[link.Start:link.End]
	if isAutoLinkedFileRef(href, label) {
		return nil
	}
	return &markdown.RenderedLink{
		Start: link.Start,
		End:   link.End,
		Open:  `<a href="` + escapeHTMLAttr(href) + `">`,
		Close: "</a>",
	}
}
