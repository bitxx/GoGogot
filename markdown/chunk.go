package markdown

import "strings"

// ChunkIR splits an IR into multiple IRs, each fitting within the byte limit.
// Style and link spans are sliced correctly across chunk boundaries so they
// can be independently rendered without broken tags.
func ChunkIR(ir IR, limit int) []IR {
	if ir.Text == "" {
		return nil
	}
	if limit <= 0 || len(ir.Text) <= limit {
		return []IR{ir}
	}

	textChunks := chunkText(ir.Text, limit)
	results := make([]IR, 0, len(textChunks))
	cursor := 0

	for i, chunk := range textChunks {
		if chunk == "" {
			continue
		}
		// Skip leading whitespace between chunks (except the first)
		if i > 0 {
			for cursor < len(ir.Text) && isWhitespace(ir.Text[cursor]) {
				cursor++
			}
		}
		start := cursor
		end := start + len(chunk)
		if end > len(ir.Text) {
			end = len(ir.Text)
		}

		results = append(results, IR{
			Text:   chunk,
			Styles: sliceStyles(ir.Styles, start, end),
			Links:  sliceLinks(ir.Links, start, end),
		})
		cursor = end
	}

	return results
}

func chunkText(text string, limit int) []string {
	var chunks []string
	for len(text) > 0 {
		if len(text) <= limit {
			chunks = append(chunks, text)
			break
		}
		cut := strings.LastIndex(text[:limit], "\n")
		if cut < limit/2 {
			cut = limit
		}
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	return chunks
}

func sliceStyles(spans []StyleSpan, start, end int) []StyleSpan {
	var result []StyleSpan
	for _, sp := range spans {
		s := maxInt(sp.Start, start)
		e := minInt(sp.End, end)
		if e > s {
			result = append(result, StyleSpan{
				Start: s - start,
				End:   e - start,
				Style: sp.Style,
			})
		}
	}
	return mergeStyles(result)
}

func sliceLinks(links []LinkSpan, start, end int) []LinkSpan {
	var result []LinkSpan
	for _, l := range links {
		s := maxInt(l.Start, start)
		e := minInt(l.End, end)
		if e > s {
			result = append(result, LinkSpan{
				Start: s - start,
				End:   e - start,
				Href:  l.Href,
			})
		}
	}
	return result
}

func mergeStyles(spans []StyleSpan) []StyleSpan {
	if len(spans) <= 1 {
		return spans
	}
	sort := make([]StyleSpan, len(spans))
	copy(sort, spans)
	sortStyleSpans(sort)

	merged := []StyleSpan{sort[0]}
	for i := 1; i < len(sort); i++ {
		prev := &merged[len(merged)-1]
		cur := sort[i]
		if prev.Style == cur.Style && cur.Start <= prev.End {
			if cur.End > prev.End {
				prev.End = cur.End
			}
		} else {
			merged = append(merged, cur)
		}
	}
	return merged
}

func sortStyleSpans(spans []StyleSpan) {
	n := len(spans)
	for i := 1; i < n; i++ {
		for j := i; j > 0; j-- {
			if spanLess(spans[j], spans[j-1]) {
				spans[j], spans[j-1] = spans[j-1], spans[j]
			} else {
				break
			}
		}
	}
}

func spanLess(a, b StyleSpan) bool {
	if a.Start != b.Start {
		return a.Start < b.Start
	}
	if a.End != b.End {
		return a.End < b.End
	}
	return a.Style < b.Style
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
