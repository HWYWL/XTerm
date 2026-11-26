package app

import (
	"fmt"
	"strings"

	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The assistant answers in Markdown; the AI tab shows it as a page: headings,
// paragraphs with bold, italic, inline code and links, lists, quotes, tables
// and code blocks with a copy button, rather than the markers as typed.

// mdKind is what a block of the answer is.
type mdKind int

const (
	mdParagraph mdKind = iota
	mdHeading
	mdCode
	mdList
	mdQuote
	mdTable
	mdRule
)

// mdBlock is one block of a rendered answer.
type mdBlock struct {
	kind mdKind
	// spans are a paragraph's, a heading's or a quote's words.
	spans []mdSpan
	// level is a heading's, 1 for a first-level one.
	level int
	// text and lang are a code block's body and language.
	text string
	lang string
	// items are a list's, ordered or not, each with its depth.
	items   []mdItem
	ordered bool
	// rows are a table's, the first the header's.
	rows [][]string
}

// mdItem is one item of a list.
type mdItem struct {
	level int
	spans []mdSpan
}

// mdSpan is a run of an answer's text with one style, and a link when it
// has a URL.
type mdSpan struct {
	text   string
	bold   bool
	italic bool
	code   bool
	url    string
}

// parseMarkdown splits src into the blocks the AI tab shows. It takes what
// a stream has written so far: an unclosed code block runs to the end.
func parseMarkdown(src string) []mdBlock {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var blocks []mdBlock
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			mark := trimmed[:3]
			lang := strings.TrimSpace(trimmed[3:])
			var body []string
			for i++; i < len(lines); i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), mark) {
					i++
					break
				}
				body = append(body, lines[i])
			}
			blocks = append(blocks, mdBlock{kind: mdCode, text: strings.Join(body, "\n"), lang: lang})
			continue
		}
		if level, text, ok := mdHeadingLine(trimmed); ok {
			blocks = append(blocks, mdBlock{kind: mdHeading, level: level, spans: parseMarkdownInline(text)})
			i++
			continue
		}
		if mdRuleLine(trimmed) {
			blocks = append(blocks, mdBlock{kind: mdRule})
			i++
			continue
		}
		if strings.Contains(trimmed, "|") && i+1 < len(lines) && mdTableSeparator(lines[i+1]) {
			rows := [][]string{mdCells(trimmed)}
			for i += 2; i < len(lines); i++ {
				row := strings.TrimSpace(lines[i])
				if row == "" || !strings.Contains(row, "|") {
					break
				}
				rows = append(rows, mdCells(row))
			}
			blocks = append(blocks, mdBlock{kind: mdTable, rows: rows})
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			var parts []string
			for ; i < len(lines); i++ {
				quoted := strings.TrimSpace(lines[i])
				if !strings.HasPrefix(quoted, ">") {
					break
				}
				parts = append(parts, strings.TrimSpace(strings.TrimPrefix(quoted, ">")))
			}
			blocks = append(blocks, mdBlock{kind: mdQuote, spans: parseMarkdownInline(strings.Join(parts, "\n"))})
			continue
		}
		if _, _, _, ok := mdListItem(line); ok {
			block := mdBlock{kind: mdList}
			for ; i < len(lines); i++ {
				level, ordered, text, ok := mdListItem(lines[i])
				if !ok {
					break
				}
				block.ordered = ordered
				block.items = append(block.items, mdItem{level: level, spans: parseMarkdownInline(text)})
			}
			blocks = append(blocks, block)
			continue
		}
		var parts []string
		for ; i < len(lines); i++ {
			text := strings.TrimSpace(lines[i])
			if text == "" || mdBlockStart(lines, i) {
				break
			}
			parts = append(parts, text)
		}
		blocks = append(blocks, mdBlock{kind: mdParagraph, spans: parseMarkdownInline(strings.Join(parts, "\n"))})
	}
	return blocks
}

// mdBlockStart says whether the line at i starts a block other than a
// paragraph, which ends the paragraph before it.
func mdBlockStart(lines []string, i int) bool {
	text := strings.TrimSpace(lines[i])
	if strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~") || strings.HasPrefix(text, ">") {
		return true
	}
	if _, _, ok := mdHeadingLine(text); ok {
		return true
	}
	if mdRuleLine(text) {
		return true
	}
	if _, _, _, ok := mdListItem(lines[i]); ok {
		return true
	}
	return strings.Contains(text, "|") && i+1 < len(lines) && mdTableSeparator(lines[i+1])
}

// mdHeadingLine reads an ATX heading, as "## Notes".
func mdHeadingLine(text string) (level int, body string, ok bool) {
	n := 0
	for n < len(text) && text[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n == len(text) || text[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(strings.TrimRight(text[n:], "#")), true
}

// mdRuleLine reads a horizontal rule, as "---" or "***".
func mdRuleLine(text string) bool {
	if len(text) < 3 {
		return false
	}
	c := text[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] != c && text[i] != ' ' {
			return false
		}
	}
	return true
}

// mdListItem reads a list item, as "- x", "1. x" or an indented one, with
// its depth in the indentation.
func mdListItem(line string) (level int, ordered bool, text string, ok bool) {
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		if line[indent] == '\t' {
			indent += 4
		} else {
			indent++
		}
	}
	level = min(indent/2, 6)
	rest := line[indent:]
	if len(rest) >= 2 && strings.ContainsRune("-*+", rune(rest[0])) && rest[1] == ' ' {
		return level, false, strings.TrimSpace(rest[2:]), true
	}
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	if n == 0 || n >= len(rest) || (rest[n] != '.' && rest[n] != ')') {
		return 0, false, "", false
	}
	if n+1 >= len(rest) || rest[n+1] != ' ' {
		return 0, false, "", false
	}
	return level, true, strings.TrimSpace(rest[n+2:]), true
}

// mdTableSeparator reads a table's row under its header, as "|---|:--:|".
func mdTableSeparator(line string) bool {
	text := strings.TrimSpace(line)
	if !strings.Contains(text, "-") {
		return false
	}
	for _, c := range text {
		if c != '|' && c != '-' && c != ':' && c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

// mdCells splits a table row into its cells.
func mdCells(line string) []string {
	text := strings.TrimSpace(line)
	text = strings.TrimPrefix(text, "|")
	text = strings.TrimSuffix(text, "|")
	cells := strings.Split(text, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// parseMarkdownInline reads the styles of a run of an answer: **bold**,
// *italic*, `code`, [links](url), and ~~struck~~ as italic.
func parseMarkdownInline(s string) []mdSpan {
	var out []mdSpan
	plain := func(text string) {
		if text == "" {
			return
		}
		out = append(out, mdSpan{text: text})
	}
	for len(s) > 0 {
		switch {
		case s[0] == '`':
			if i := strings.IndexByte(s[1:], '`'); i >= 0 {
				out = append(out, mdSpan{text: s[1 : 1+i], code: true})
				s = s[i+2:]
				continue
			}
		case strings.HasPrefix(s, "**") || strings.HasPrefix(s, "__"):
			mark := s[:2]
			if i := strings.Index(s[2:], mark); i >= 0 {
				for _, sp := range parseMarkdownInline(s[2 : 2+i]) {
					sp.bold = true
					out = append(out, sp)
				}
				s = s[i+4:]
				continue
			}
		case strings.HasPrefix(s, "~~"):
			if i := strings.Index(s[2:], "~~"); i >= 0 {
				for _, sp := range parseMarkdownInline(s[2 : 2+i]) {
					sp.italic = true
					out = append(out, sp)
				}
				s = s[i+4:]
				continue
			}
		case s[0] == '*':
			if i := strings.IndexByte(s[1:], '*'); i >= 0 {
				for _, sp := range parseMarkdownInline(s[1 : 1+i]) {
					sp.italic = true
					out = append(out, sp)
				}
				s = s[i+2:]
				continue
			}
		case s[0] == '[':
			if end := strings.Index(s, "]("); end > 0 {
				if close := strings.IndexByte(s[end+2:], ')'); close > 0 {
					out = append(out, mdSpan{text: s[1:end], url: s[end+2 : end+2+close]})
					s = s[end+2+close+1:]
					continue
				}
			}
		}
		i := 1
		for i < len(s) && !mdSpecial(s[i]) {
			i++
		}
		plain(s[:i])
		s = s[i:]
	}
	// An unclosed marker leaves its text in runs, as "*" then "*bo".
	return mergeSpans(out)
}

// mergeSpans joins neighbouring runs of the same style, which a marker that
// never closed leaves apart.
func mergeSpans(spans []mdSpan) []mdSpan {
	out := spans[:0]
	for _, sp := range spans {
		if n := len(out); n > 0 && out[n-1].bold == sp.bold && out[n-1].italic == sp.italic &&
			out[n-1].code == sp.code && out[n-1].url == sp.url {
			out[n-1].text += sp.text
			continue
		}
		out = append(out, sp)
	}
	return out
}

// mdSpecial says whether b may start a run's style.
func mdSpecial(b byte) bool {
	return b == '`' || b == '*' || b == '_' || b == '~' || b == '['
}

// aiAnswer shows t's answer as the page it is written as, parsing it again
// only when the answer grew.
func (a *App) aiAnswer(c *ui.Context, t *aiTurn) {
	if t.renderedFrom != t.answer {
		t.blocks = parseMarkdown(t.answer)
		t.renderedFrom = t.answer
	}
	ui.Column(c).Gap(6).Selectable().Children(func() {
		for _, b := range t.blocks {
			a.aiBlock(c, b)
		}
	})
}

// aiBlock shows one block of an answer.
func (a *App) aiBlock(c *ui.Context, b mdBlock) {
	switch b.kind {
	case mdHeading:
		sizes := [...]float32{15.5, 14.5, 13.5, 13, 12.5, 12}
		size := sizes[min(max(b.level, 1), len(sizes))-1]
		spans := append([]mdSpan(nil), b.spans...)
		for i := range spans {
			spans[i].bold = true
		}
		ui.Column(c).Padding(4, 0).Children(func() {
			a.aiInline(c, spans, size)
		})
	case mdCode:
		ui.Column(c).Gap(4).Padding(8, 10).Radius(6).Background(theme.Surface).
			BorderWidth(1).BorderColor(theme.Border).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if b.lang != "" {
					ui.Text(c, b.lang).FontSize(10).TextColor(theme.TextMuted).Grow(1).SingleLine()
				} else {
					ui.Spacer(c)
				}
				if ui.Button(c, locales.Copy()).FontSize(10).Clicked() {
					a.copyToClipboard(c, b.text)
				}
			})
			ui.Text(c, b.text).Font("monospace").FontSize(11.5).Wrap()
		})
	case mdList:
		ui.Column(c).Gap(4).Children(func() {
			for i, item := range b.items {
				marker := "•"
				if b.ordered {
					marker = fmt.Sprintf("%d.", i+1)
				}
				ui.Row(c).Gap(8).PaddingStart(float32(item.level) * 14).AlignItems(ui.Start).Children(func() {
					ui.Text(c, marker).FontSize(12).TextColor(theme.TextMuted).Shrink(0)
					a.aiInline(c, item.spans, 12).Grow(1).MinWidth(0)
				})
			}
		})
	case mdQuote:
		ui.Row(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Width(3).Radius(2).Background(theme.Accent).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				a.aiInlineMuted(c, b.spans)
			})
		})
	case mdTable:
		ui.Column(c).Gap(0).BorderWidth(1).BorderColor(theme.Border).Radius(6).Children(func() {
			for i, row := range b.rows {
				row := row
				header := i == 0
				line := ui.Row(c).Gap(12).Padding(6, 8)
				if i > 0 {
					line.BorderWidth(1, 0, 0, 0).BorderColor(theme.Border)
				}
				line.Children(func() {
					for _, cell := range row {
						text := ui.Text(c, cell).FontSize(11.5).Grow(1).MinWidth(0).Wrap()
						if header {
							text.Bold().TextColor(theme.TextMuted)
						}
					}
				})
			}
		})
	case mdRule:
		ui.Box(c).Height(1).Background(theme.Border)
	default:
		ui.Column(c).Children(func() {
			a.aiInline(c, b.spans, 12)
		})
	}
}

// aiInline shows spans of text wrapped as a paragraph, in size.
func (a *App) aiInline(c *ui.Context, spans []mdSpan, size float32) ui.Element {
	linked := false
	for _, sp := range spans {
		if sp.url != "" && sp.text != "" {
			linked = true
			break
		}
	}
	if !linked {
		return ui.RichText(c, mdUISpans(spans)...).FontSize(size).Wrap()
	}
	// A link keeps its words clickable: the paragraph is built of text
	// elements, which continue each other.
	return ui.RichText(c).FontSize(size).Wrap().Children(func() {
		for _, sp := range spans {
			switch {
			case sp.url != "" && sp.text != "":
				ui.Link(c, sp.text, sp.url)
			case sp.code:
				ui.Text(c, sp.text).Font("monospace").FontSize(size - 0.5).
					Background(theme.SurfacePressed).Radius(3)
			default:
				text := ui.Text(c, sp.text)
				if sp.bold {
					text.Bold()
				}
				if sp.italic {
					text.Italic()
				}
			}
		}
	})
}

// aiInlineMuted is a quote's words, quieter than an answer's.
func (a *App) aiInlineMuted(c *ui.Context, spans []mdSpan) {
	ui.RichText(c, mdUISpans(spans)...).FontSize(12).Wrap().
		TextColor(theme.TextMuted).Grow(1).MinWidth(0)
}

// mdUISpans turns an answer's spans into rich text spans.
func mdUISpans(spans []mdSpan) []ui.Span {
	out := make([]ui.Span, 0, len(spans))
	for _, sp := range spans {
		u := ui.Span{Text: sp.text}
		if sp.bold {
			u.Weight = 650
		}
		if sp.italic {
			u.Italic = true
		}
		if sp.code {
			u.Font = "monospace"
			u.Size = 11.5
			u.Background = theme.SurfacePressed
			u.Radius = 3
			u.PaddingX = 3
		}
		out = append(out, u)
	}
	return out
}
