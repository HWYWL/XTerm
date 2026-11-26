package app

import (
	"encoding/json"
	"strings"
	"testing"

	"finallyshell/locales"
)

// The blocks of an answer are read as the page shows them.
func TestParseMarkdownBlocks(t *testing.T) {
	blocks := parseMarkdown("# One\n\nText **b** and `c` and [l](https://x.test).\n\n- a\n  - b\n\n1. x\n2. y\n\n```sh\nls -l\n```\n\n> q\n\n| h | i |\n|---|---|\n| 1 | 2 |\n\n---\n")
	want := []mdKind{mdHeading, mdParagraph, mdList, mdList, mdCode, mdQuote, mdTable, mdRule}
	if len(blocks) != len(want) {
		t.Fatalf("%d blocks, want %d", len(blocks), len(want))
	}
	for i, kind := range want {
		if blocks[i].kind != kind {
			t.Fatalf("block %d is %d, want %d", i, blocks[i].kind, kind)
		}
	}
	if blocks[0].level != 1 || len(blocks[0].spans) == 0 || blocks[0].spans[0].text != "One" {
		t.Errorf("the heading: %+v", blocks[0])
	}
	if spans := blocks[1].spans; !hasSpan(spans, "b", func(s mdSpan) bool { return s.bold }) ||
		!hasSpan(spans, "c", func(s mdSpan) bool { return s.code }) ||
		!hasSpan(spans, "l", func(s mdSpan) bool { return s.url == "https://x.test" }) {
		t.Errorf("the paragraph: %+v", spans)
	}
	if blocks[2].ordered || len(blocks[2].items) != 2 || blocks[2].items[1].level != 1 {
		t.Errorf("the list: %+v", blocks[2])
	}
	if !blocks[3].ordered || len(blocks[3].items) != 2 {
		t.Errorf("the ordered list: %+v", blocks[3])
	}
	if blocks[4].lang != "sh" || blocks[4].text != "ls -l" {
		t.Errorf("the code: %+v", blocks[4])
	}
	if len(blocks[6].rows) != 2 || blocks[6].rows[1][1] != "2" {
		t.Errorf("the table: %+v", blocks[6])
	}
}

// An answer still streaming ends inside its code block, and keeps what it
// has.
func TestParseMarkdownStreaming(t *testing.T) {
	blocks := parseMarkdown("```go\nx := 1")
	if len(blocks) != 1 || blocks[0].kind != mdCode || blocks[0].text != "x := 1" {
		t.Errorf("the open code block: %+v", blocks)
	}
	blocks = parseMarkdown("**bo")
	if len(blocks) != 1 || len(blocks[0].spans) != 1 || blocks[0].spans[0].text != "**bo" {
		t.Errorf("the open bold: %+v", blocks)
	}
}

// hasSpan says whether spans hold text that matches, as the style says.
func hasSpan(spans []mdSpan, text string, match func(mdSpan) bool) bool {
	for _, s := range spans {
		if s.text == text && match(s) {
			return true
		}
	}
	return false
}

// A long answer scrolls in the list: the box the next question is typed in
// stays inside the panel, not pushed below it.
func TestAIComposerStaysInPanel(t *testing.T) {
	answer := strings.Repeat("这是一段很长的回答，用来把列表撑得比面板还高。\n\n", 60)
	srv := aiServer(t, aiPiece(answer))
	a, p, tt, s := aiApp(t, srv.URL)
	// A short window: the panel has little room, and the list above the
	// composer scrolls rather than pushing it out.
	tt.SetSize(1280, 620)
	a.terminalPct = 75
	s.ai.input = "hi"
	a.aiSend(s)
	p.until(t, "the answer", func() bool { return !s.ai.busy && len(s.ai.turns) == 1 })
	tt.Frame()
	in, ok := tt.Find(locales.AiAskPlaceholder())
	if !ok {
		t.Fatalf("the composer is not shown; texts %q", tt.Texts())
	}
	if in.Y+in.H > 620 {
		t.Errorf("the composer ends at %v, below the window's 620", in.Y+in.H)
	}
	newChat, ok := tt.Find(locales.AiNewChat())
	if !ok {
		t.Fatalf("the new chat button is not shown; texts %q", tt.Texts())
	}
	if newChat.Y+newChat.H > 620 {
		t.Errorf("the composer's buttons end at %v, below the window's 620", newChat.Y+newChat.H)
	}
	saveShot(t, tt, "ai-composer-panel-en.png")
}

// aiPiece is a piece of an answer, as the server streams it.
func aiPiece(content string) string {
	b, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"delta": map[string]any{"content": content}}},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// The AI tab shows the answer as a page: headings, lists, a code block with
// its copy button, a quote and a table, not the markers as typed.
func TestAIChatRendersMarkdown(t *testing.T) {
	answer := "## 诊断\n\n先看 **磁盘** 和 `du -sh`：\n\n- 根分区\n- 日志目录\n\n```sh\ndf -h\n```\n\n> 只读时也要小心\n\n| 挂载点 | 用量 |\n|---|---|\n| / | 62% |\n\n见 [文档](https://example.com/x)。"
	srv := aiServer(t, aiPiece(answer))
	a, p, tt, s := aiApp(t, srv.URL)
	// The panel takes most of the window, as it would be dragged up.
	a.terminalPct = 30
	s.ai.input = "看看磁盘"
	a.aiSend(s)
	p.until(t, "the answer", func() bool { return !s.ai.busy && len(s.ai.turns) == 1 })
	tt.Frame()
	for _, want := range []string{"诊断", "磁盘", "du -sh", "根分区", "日志目录", "df -h", "只读时也要小心", "挂载点", "62%"} {
		if !tt.HasText(want) {
			t.Fatalf("%q is not shown; texts %q", want, tt.Texts())
		}
	}
	for _, marker := range []string{"## ", "**", "```", "|---"} {
		if tt.HasText(marker) {
			t.Errorf("the page shows the marker %q", marker)
		}
	}
	saveShot(t, tt, "ai-chat-markdown-en.png")
}
