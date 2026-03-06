package markdown

import (
	"sort"
	"strings"
)

type StyleMarker struct {
	Open  string
	Close string
}

type RenderedLink struct {
	Start int
	End   int
	Open  string
	Close string
}

type RenderOptions struct {
	StyleMarkers map[Style]StyleMarker
	EscapeText   func(string) string
	BuildLink    func(link LinkSpan, text string) *RenderedLink
}

var styleOrder = map[Style]int{
	Blockquote:    0,
	CodeBlock:     1,
	Code:          2,
	Bold:          3,
	Italic:        4,
	Strikethrough: 5,
}

func styleRank(s Style) int {
	if r, ok := styleOrder[s]; ok {
		return r
	}
	return 99
}

type boundary struct {
	pos   int
	close string
	end   int
}

type opening struct {
	end   int
	open  string
	close string
	rank  int
	index int
}

// Render converts an IR into a formatted string using the given markers.
// It walks all style/link boundaries in LIFO order, inserting open/close
// tags around escaped text — the same approach as openclaw's renderMarkdownWithMarkers.
func Render(ir IR, opts RenderOptions) string {
	text := ir.Text
	if text == "" {
		return ""
	}

	filtered := make([]StyleSpan, 0, len(ir.Styles))
	for _, sp := range ir.Styles {
		if _, ok := opts.StyleMarkers[sp.Style]; ok && sp.End > sp.Start {
			filtered = append(filtered, sp)
		}
	}

	points := collectBoundaryPoints(filtered, ir.Links, opts, text)
	sort.Ints(points)
	points = dedupInts(points)

	var stack []boundary
	var out strings.Builder
	out.Grow(len(text) + len(text)/4)

	startsAt := buildStyleStartMap(filtered, opts)
	linkStartsAt := buildLinkStartMap(ir.Links, opts, text)

	for i, pos := range points {
		// Close elements ending at this position (LIFO)
		for len(stack) > 0 && stack[len(stack)-1].end == pos {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			out.WriteString(top.close)
		}

		// Collect openings at this position
		var openings []opening
		idx := 0
		if links, ok := linkStartsAt[pos]; ok {
			for _, rl := range links {
				openings = append(openings, opening{
					end: rl.End, open: rl.Open, close: rl.Close,
					rank: -1, index: idx,
				})
				idx++
			}
		}
		if spans, ok := startsAt[pos]; ok {
			for _, sp := range spans {
				m := opts.StyleMarkers[sp.Style]
				openings = append(openings, opening{
					end: sp.End, open: m.Open, close: m.Close,
					rank: styleRank(sp.Style), index: idx,
				})
				idx++
			}
		}

		// Sort: wider spans first (larger end), then by rank
		sort.Slice(openings, func(a, b int) bool {
			if openings[a].end != openings[b].end {
				return openings[a].end > openings[b].end
			}
			if openings[a].rank != openings[b].rank {
				return openings[a].rank < openings[b].rank
			}
			return openings[a].index < openings[b].index
		})

		for _, o := range openings {
			out.WriteString(o.open)
			stack = append(stack, boundary{close: o.close, end: o.end})
		}

		// Write escaped text between this point and the next
		if i+1 < len(points) {
			next := points[i+1]
			if next > pos {
				out.WriteString(opts.EscapeText(text[pos:next]))
			}
		}
	}

	return out.String()
}

func collectBoundaryPoints(styles []StyleSpan, links []LinkSpan, opts RenderOptions, text string) []int {
	set := map[int]struct{}{
		0:          {},
		len(text): {},
	}
	for _, sp := range styles {
		set[sp.Start] = struct{}{}
		set[sp.End] = struct{}{}
	}
	if opts.BuildLink != nil {
		for _, l := range links {
			rl := opts.BuildLink(l, text)
			if rl != nil {
				set[rl.Start] = struct{}{}
				set[rl.End] = struct{}{}
			}
		}
	}
	pts := make([]int, 0, len(set))
	for p := range set {
		pts = append(pts, p)
	}
	return pts
}

func buildStyleStartMap(styles []StyleSpan, opts RenderOptions) map[int][]StyleSpan {
	m := make(map[int][]StyleSpan)
	for _, sp := range styles {
		if _, ok := opts.StyleMarkers[sp.Style]; ok {
			m[sp.Start] = append(m[sp.Start], sp)
		}
	}
	for _, spans := range m {
		sort.Slice(spans, func(i, j int) bool {
			if spans[i].End != spans[j].End {
				return spans[i].End > spans[j].End
			}
			return styleRank(spans[i].Style) < styleRank(spans[j].Style)
		})
	}
	return m
}

func buildLinkStartMap(links []LinkSpan, opts RenderOptions, text string) map[int][]RenderedLink {
	if opts.BuildLink == nil {
		return nil
	}
	m := make(map[int][]RenderedLink)
	for _, l := range links {
		rl := opts.BuildLink(l, text)
		if rl != nil {
			m[rl.Start] = append(m[rl.Start], *rl)
		}
	}
	return m
}

func dedupInts(sorted []int) []int {
	if len(sorted) <= 1 {
		return sorted
	}
	j := 0
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[j] {
			j++
			sorted[j] = sorted[i]
		}
	}
	return sorted[:j+1]
}
