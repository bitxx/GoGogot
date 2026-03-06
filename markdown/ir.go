package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

type Style string

const (
	Bold          Style = "bold"
	Italic        Style = "italic"
	Strikethrough Style = "strikethrough"
	Code          Style = "code"
	CodeBlock     Style = "code_block"
	Blockquote    Style = "blockquote"
)

type StyleSpan struct {
	Start int
	End   int
	Style Style
}

type LinkSpan struct {
	Start int
	End   int
	Href  string
}

type IR struct {
	Text   string
	Styles []StyleSpan
	Links  []LinkSpan
}

type parseState struct {
	buf        strings.Builder
	styles     []StyleSpan
	links      []LinkSpan
	openStyles []openStyle
	openLinks  []openLink
	listStack  []listState
}

type openStyle struct {
	style Style
	start int
}

type openLink struct {
	href  string
	start int
}

type listState struct {
	ordered bool
	index   int
}

var mdParser goldmark.Markdown

func init() {
	mdParser = goldmark.New(
		goldmark.WithExtensions(
			extension.Strikethrough,
			extension.Table,
		),
	)
}

func Parse(markdown string) IR {
	source := []byte(markdown)
	doc := mdParser.Parser().Parse(text.NewReader(source))

	s := &parseState{}
	s.walkNode(doc, source, 0)
	s.closeRemaining()

	result := strings.TrimRight(s.buf.String(), "\n ")

	return IR{
		Text:   result,
		Styles: clampSpans(s.styles, len(result)),
		Links:  clampLinks(s.links, len(result)),
	}
}

func (s *parseState) pos() int {
	return s.buf.Len()
}

func (s *parseState) write(text string) {
	s.buf.WriteString(text)
}

func (s *parseState) pushStyle(style Style) {
	s.openStyles = append(s.openStyles, openStyle{style: style, start: s.pos()})
}

func (s *parseState) popStyle(style Style) {
	for i := len(s.openStyles) - 1; i >= 0; i-- {
		if s.openStyles[i].style == style {
			start := s.openStyles[i].start
			end := s.pos()
			s.openStyles = append(s.openStyles[:i], s.openStyles[i+1:]...)
			if end > start {
				s.styles = append(s.styles, StyleSpan{Start: start, End: end, Style: style})
			}
			return
		}
	}
}

func (s *parseState) pushLink(href string) {
	s.openLinks = append(s.openLinks, openLink{href: href, start: s.pos()})
}

func (s *parseState) popLink() {
	if len(s.openLinks) == 0 {
		return
	}
	top := s.openLinks[len(s.openLinks)-1]
	s.openLinks = s.openLinks[:len(s.openLinks)-1]
	end := s.pos()
	if end > top.start && top.href != "" {
		s.links = append(s.links, LinkSpan{Start: top.start, End: end, Href: top.href})
	}
}

func (s *parseState) closeRemaining() {
	for i := len(s.openStyles) - 1; i >= 0; i-- {
		o := s.openStyles[i]
		end := s.pos()
		if end > o.start {
			s.styles = append(s.styles, StyleSpan{Start: o.start, End: end, Style: o.style})
		}
	}
	s.openStyles = nil
}

func (s *parseState) listPrefix() string {
	if len(s.listStack) == 0 {
		return ""
	}
	top := &s.listStack[len(s.listStack)-1]
	top.index++
	indent := strings.Repeat("  ", max(0, len(s.listStack)-1))
	if top.ordered {
		return fmt.Sprintf("%s%d. ", indent, top.index)
	}
	return indent + "• "
}

func (s *parseState) inList() bool {
	return len(s.listStack) > 0
}

func (s *parseState) walkNode(n ast.Node, source []byte, depth int) {
	switch node := n.(type) {
	case *ast.Document:
		s.walkChildren(n, source, depth)

	case *ast.Paragraph:
		s.walkChildren(n, source, depth)
		if !s.inList() {
			s.write("\n\n")
		}

	case *ast.Heading:
		s.pushStyle(Bold)
		s.walkChildren(n, source, depth)
		s.popStyle(Bold)
		s.write("\n\n")

	case *ast.Emphasis:
		if node.Level == 2 {
			s.pushStyle(Bold)
			s.walkChildren(n, source, depth)
			s.popStyle(Bold)
		} else {
			s.pushStyle(Italic)
			s.walkChildren(n, source, depth)
			s.popStyle(Italic)
		}

	case *ast.CodeSpan:
		start := s.pos()
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				s.write(string(t.Segment.Value(source)))
			}
		}
		end := s.pos()
		if end > start {
			s.styles = append(s.styles, StyleSpan{Start: start, End: end, Style: Code})
		}

	case *ast.FencedCodeBlock, *ast.CodeBlock:
		start := s.pos()
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			s.write(string(seg.Value(source)))
		}
		text := s.buf.String()[start:]
		if !strings.HasSuffix(text, "\n") {
			s.write("\n")
		}
		end := s.pos()
		if end > start {
			s.styles = append(s.styles, StyleSpan{Start: start, End: end, Style: CodeBlock})
		}
		if !s.inList() {
			s.write("\n")
		}

	case *ast.Blockquote:
		s.pushStyle(Blockquote)
		s.walkChildren(n, source, depth)
		s.popStyle(Blockquote)

	case *ast.Link:
		href := string(node.Destination)
		s.pushLink(href)
		s.walkChildren(n, source, depth)
		s.popLink()

	case *ast.AutoLink:
		href := string(node.URL(source))
		label := string(node.Label(source))
		start := s.pos()
		s.write(label)
		end := s.pos()
		if end > start {
			s.links = append(s.links, LinkSpan{Start: start, End: end, Href: href})
		}

	case *ast.Image:
		s.write(string(node.Text(source)))

	case *ast.Text:
		s.write(string(node.Segment.Value(source)))
		if node.SoftLineBreak() || node.HardLineBreak() {
			s.write("\n")
		}

	case *ast.String:
		s.write(string(node.Value))

	case *ast.List:
		if s.inList() {
			s.write("\n")
		}
		s.listStack = append(s.listStack, listState{
			ordered: node.IsOrdered(),
			index:   node.Start - 1,
		})
		s.walkChildren(n, source, depth)
		s.listStack = s.listStack[:len(s.listStack)-1]
		if !s.inList() {
			s.write("\n")
		}

	case *ast.ListItem:
		s.write(s.listPrefix())
		s.walkChildren(n, source, depth)
		if !strings.HasSuffix(s.buf.String(), "\n") {
			s.write("\n")
		}

	case *ast.ThematicBreak:
		s.write("───\n\n")

	case *ast.HTMLBlock:
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			s.write(string(seg.Value(source)))
		}

	case *ast.RawHTML:
		for i := 0; i < n.ChildCount(); i++ {
			segs := node.Segments
			for j := 0; j < segs.Len(); j++ {
				seg := segs.At(j)
				s.write(string(seg.Value(source)))
			}
		}

	case *east.Strikethrough:
		s.pushStyle(Strikethrough)
		s.walkChildren(n, source, depth)
		s.popStyle(Strikethrough)

	case *east.Table:
		s.renderTableAsBullets(n, source, depth)

	default:
		s.walkChildren(n, source, depth)
	}
}

func (s *parseState) walkChildren(n ast.Node, source []byte, depth int) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		s.walkNode(c, source, depth+1)
	}
}

func (s *parseState) renderTableAsBullets(table ast.Node, source []byte, depth int) {
	var headers []string
	var rows [][]string

	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.(type) {
		case *east.TableHeader:
			for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
				headers = append(headers, s.extractCellText(cell, source))
			}
		case *east.TableRow:
			var rowCells []string
			for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
				rowCells = append(rowCells, s.extractCellText(cell, source))
			}
			rows = append(rows, rowCells)
		}
	}

	if len(headers) == 0 && len(rows) == 0 {
		return
	}

	useFirstColAsLabel := len(headers) > 1 && len(rows) > 0

	if useFirstColAsLabel {
		for _, row := range rows {
			if len(row) == 0 {
				continue
			}
			start := s.pos()
			s.write(row[0])
			end := s.pos()
			if end > start {
				s.styles = append(s.styles, StyleSpan{Start: start, End: end, Style: Bold})
			}
			s.write("\n")

			for i := 1; i < len(row); i++ {
				s.write("• ")
				if i < len(headers) && headers[i] != "" {
					s.write(headers[i])
					s.write(": ")
				}
				s.write(row[i])
				s.write("\n")
			}
			s.write("\n")
		}
	} else {
		for _, row := range rows {
			for i, cell := range row {
				s.write("• ")
				if i < len(headers) && headers[i] != "" {
					s.write(headers[i])
					s.write(": ")
				}
				s.write(cell)
				s.write("\n")
			}
			s.write("\n")
		}
	}
}

func (s *parseState) extractCellText(cell ast.Node, source []byte) string {
	var b strings.Builder
	s.extractTextRecursive(cell, source, &b)
	return strings.TrimSpace(b.String())
}

func (s *parseState) extractTextRecursive(n ast.Node, source []byte, b *strings.Builder) {
	if t, ok := n.(*ast.Text); ok {
		b.Write(t.Segment.Value(source))
		return
	}
	if cs, ok := n.(*ast.CodeSpan); ok {
		_ = cs
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				b.Write(t.Segment.Value(source))
			}
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		s.extractTextRecursive(c, source, b)
	}
}

func clampSpans(spans []StyleSpan, maxLen int) []StyleSpan {
	var result []StyleSpan
	for _, sp := range spans {
		start := clampInt(sp.Start, 0, maxLen)
		end := clampInt(sp.End, start, maxLen)
		if end > start {
			result = append(result, StyleSpan{Start: start, End: end, Style: sp.Style})
		}
	}
	return result
}

func clampLinks(links []LinkSpan, maxLen int) []LinkSpan {
	var result []LinkSpan
	for _, l := range links {
		start := clampInt(l.Start, 0, maxLen)
		end := clampInt(l.End, start, maxLen)
		if end > start {
			result = append(result, LinkSpan{Start: start, End: end, Href: l.Href})
		}
	}
	return result
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
