package app

import (
	"regexp"
	"sort"
	"strconv"
	"sync"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The terminal's text highlighting, as QuickTerm's settings have it: rules
// of regular expressions, which a line of output is scanned with, and the
// color each match takes.
//
// The rules, their page and their file are here. Coloring the cells is the
// terminal's, and mygo's terminal, which draws them from Ghostty's state,
// takes no per-cell color yet: the rules wait in highlight.json for the
// release that does (see highlightPage).

// hlSpan is a run of a line a rule matched: its bytes in the line, and the
// rule's place in the list, whose color it takes.
type hlSpan struct {
	start, end int
	rule       int
}

// highlighter keeps the rules' expressions: a line of output meets every
// rule, and the same patterns are compiled again for every line otherwise.
type highlighter struct {
	mu    sync.Mutex
	exprs map[string]*regexp.Regexp
}

// spans are the runs of line the rules match, in the order they show. A
// run an earlier rule took is not taken by a later one, as in QuickTerm:
// the number rule comes last, and leaves the digits of a file size or a
// time to them.
func (h *highlighter) spans(line string, rules []config.HighlightRule) []hlSpan {
	if line == "" {
		return nil
	}
	var spans []hlSpan
	for i, r := range rules {
		if !r.Enabled || r.Pattern == "" {
			continue
		}
		expr := h.compile(r)
		if expr == nil {
			continue
		}
		for _, m := range expr.FindAllStringIndex(line, -1) {
			if m[0] == m[1] || overlaps(spans, m[0], m[1]) {
				continue
			}
			spans = append(spans, hlSpan{start: m[0], end: m[1], rule: i})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans
}

// compile is the expression of a rule, nil for one that does not compile:
// a mis-typed pattern costs its rule, not the panel.
func (h *highlighter) compile(r config.HighlightRule) *regexp.Regexp {
	src := highlightPattern(r)
	h.mu.Lock()
	defer h.mu.Unlock()
	if expr, ok := h.exprs[src]; ok {
		return expr
	}
	expr, err := regexp.Compile(src)
	if err != nil {
		expr = nil
	}
	if h.exprs == nil {
		h.exprs = map[string]*regexp.Regexp{}
	}
	h.exprs[src] = expr
	return expr
}

// highlightPattern is the expression source of a rule, its case flag and
// all.
func highlightPattern(r config.HighlightRule) string {
	if r.IgnoreCase {
		return "(?i)" + r.Pattern
	}
	return r.Pattern
}

// overlaps says whether start, end meets one of spans.
func overlaps(spans []hlSpan, start, end int) bool {
	for _, s := range spans {
		if start < s.end && s.start < end {
			return true
		}
	}
	return false
}

// highlightName is what a rule is called: a preset's is the app's, in the
// user's language, and a custom one's is the user's.
func highlightName(r config.HighlightRule) string {
	if !r.Builtin {
		return r.Name
	}
	switch r.ID {
	case "hl-error":
		return locales.HighlightPresetError()
	case "hl-warn":
		return locales.HighlightPresetWarn()
	case "hl-info":
		return locales.HighlightPresetInfo()
	case "hl-debug":
		return locales.HighlightPresetDebug()
	case "hl-perm":
		return locales.HighlightPresetPermissions()
	case "hl-ts":
		return locales.HighlightPresetTime()
	case "hl-ipv4":
		return locales.HighlightPresetIP()
	case "hl-url":
		return locales.HighlightPresetLink()
	case "hl-size":
		return locales.HighlightPresetSize()
	case "hl-kw":
		return locales.HighlightPresetKeywords()
	case "hl-builtin":
		return locales.HighlightPresetCommands()
	case "hl-num":
		return locales.HighlightPresetNumber()
	}
	return r.Name
}

// highlightColor is a rule's color, the text's when the string is no color.
func highlightColor(s string) ui.Color {
	if len(s) != 7 || s[0] != '#' {
		return theme.Text
	}
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return theme.Text
	}
	return ui.RGBA(uint8(n>>16), uint8(n>>8), uint8(n), 1)
}

// saveHighlight keeps the rules, and redraws the pages that show them.
func (a *App) saveHighlight() {
	_ = config.SaveHighlightRules(a.highlight)
	a.redraw()
}
