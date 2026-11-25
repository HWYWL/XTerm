package app

import (
	"strings"

	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// aiPanel is the AI tab of the bottom panel: the turns of the session's
// conversation, the command the assistant waits to run, and the box the
// next question is typed in. Each session has a tab and a conversation of
// its own, so that the tabs of different hosts do not mix.
func (a *App) aiPanel(c *ui.Context) {
	s := a.connSession()
	if s == nil {
		return
	}
	st := &s.ai
	a.ensureAIKey()
	if st.openReasoning == nil {
		st.openReasoning = map[int]bool{}
	}
	if !a.settings.AI.Enabled {
		a.aiTurnedOff(c)
		return
	}
	ui.Column(c).Key(s.ID).Fill().Children(func() {
		a.aiHeader(c)
		a.aiTurns(c, s)
		if st.pending != nil {
			a.aiPendingRow(c, s)
		}
		a.aiComposer(c, s)
	})
}

// aiTurnedOff is the page before the assistant is turned on.
func (a *App) aiTurnedOff(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		widgets.Muted(c, locales.AiEnableFirst()).Wrap()
		if ui.PrimaryButton(c, locales.AiOpenSettings()).Clicked() {
			a.openAISettings()
		}
	})
}

// aiHeader is the top of the page: the model that answers, and the way to
// the options.
func (a *App) aiHeader(c *ui.Context) {
	label := a.settings.AI.ModelLabel()
	if label == "" {
		label = locales.AiNoModel()
	}
	ui.Row(c).Padding(8, 10).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		widgets.Muted(c, label).FontSize(11).SingleLine().Grow(1).MinWidth(0)
		if ui.Button(c, locales.AiSettingsShort()).FontSize(10).Clicked() {
			a.openAISettings()
		}
	})
}

// aiTurns is s's conversation, from the oldest turn down.
func (a *App) aiTurns(c *ui.Context, s *Session) {
	st := &s.ai
	if len(st.turns) == 0 {
		ui.Column(c).Grow(1).Padding(16).Gap(6).Children(func() {
			widgets.Muted(c, locales.AiEmpty()).Wrap()
			widgets.Muted(c, locales.AiEmptyHint()).Wrap().FontSize(11)
		})
		return
	}
	st.list.FollowEnd = true
	st.list.Key = func(i int) any { return i }
	ui.List(c, &st.list, len(st.turns), func(i int) { a.aiTurnRow(c, s, i) }).Grow(1).MinHeight(0)
}

// aiTurnRow is one question and its answer, with what it thought before the
// answer, and what can be done with it.
func (a *App) aiTurnRow(c *ui.Context, s *Session, i int) {
	st := &s.ai
	t := &st.turns[i]
	ui.Column(c).Gap(6).Padding(8, 10).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, t.prompt).Wrap().Selectable().FontSize(12).TextColor(theme.TextMuted)
		if t.reasoning != "" {
			open := st.openReasoning[i]
			label := locales.AiReasoning()
			if open {
				label = locales.AiHideReasoning()
			}
			if ui.Button(c, label).FontSize(10).Clicked() {
				st.openReasoning[i] = !open
			}
			if open {
				ui.Text(c, t.reasoning).Wrap().Selectable().FontSize(11).TextColor(theme.TextMuted)
			}
		}
		if t.answer != "" {
			ui.Text(c, t.answer).Wrap().Selectable().FontSize(12)
		}
		if t.stopped && t.answer == "" {
			widgets.Muted(c, locales.AiStopped()).FontSize(11)
		}
		if t.failed != "" {
			ui.Text(c, t.failed).Wrap().TextColor(theme.Danger).FontSize(11)
		}
		if st.busy && i == len(st.turns)-1 {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).FontSize(11)
				widgets.Muted(c, locales.AiThinking()).FontSize(11)
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, locales.AiCopy()).FontSize(10).Disabled(t.answer == "").Clicked() {
				a.copyToClipboard(c, t.answer)
			}
			if ui.Button(c, locales.AiInsert()).FontSize(10).Disabled(t.answer == "").Clicked() {
				a.aiInsert(s, t.answer)
			}
			if i == len(st.turns)-1 && !st.busy && t.answer != "" &&
				ui.Button(c, locales.AiRegenerate()).FontSize(10).Clicked() {
				a.aiRegenerate(s)
			}
		})
	})
}

// aiPendingRow is the command the assistant asked to run, and whether to.
func (a *App) aiPendingRow(c *ui.Context, s *Session) {
	p := s.ai.pending
	ui.Column(c).Gap(6).Padding(8, 10).Background(theme.Surface).
		BorderWidth(1, 0, 1, 0).BorderColor(theme.Border).Children(func() {
		ui.Text(c, locales.AiRunCommand()).Wrap().FontSize(11)
		ui.Text(c, p.cmd).Wrap().Selectable().FontSize(11.5)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, locales.AiAllow()).FontSize(11).Clicked() {
				a.aiAllow(s)
			}
			if ui.Button(c, locales.AiDeny()).FontSize(11).Clicked() {
				a.aiDeny(s)
			}
		})
	})
}

// aiComposer is the box the next question is typed in, with the button that
// sends it at its end, and the one that starts another conversation: Enter
// sends, and Ctrl+Enter breaks the line.
func (a *App) aiComposer(c *ui.Context, s *Session) {
	st := &s.ai
	ui.Column(c).Gap(6).Padding(8).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		if st.err != "" {
			ui.Text(c, st.err).Wrap().FontSize(11).TextColor(theme.Danger)
		}
		var in ui.Element
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			in = ui.TextArea(c, &st.input).Grow(1).MinWidth(0).Lines(2, 5).FontSize(12).
				Label(locales.AiAskPlaceholder()).Placeholder(locales.AiAskPlaceholder())
			ui.Column(c).Gap(6).Shrink(0).Children(func() {
				if st.busy {
					if ui.Button(c, locales.AiStop()).FontSize(11).Clicked() {
						a.aiStop(s)
					}
				} else if ui.PrimaryButton(c, locales.AiSend()).FontSize(11).
					Disabled(strings.TrimSpace(st.input) == "").Clicked() {
					a.aiSend(s)
				}
				if ui.Button(c, locales.AiNewChat()).FontSize(10).
					Disabled(!st.busy && len(st.turns) == 0).Clicked() {
					a.aiClear(s)
				}
			})
		})
		in.OnTextKey(func(k ui.TextKey) bool { return a.aiKey(s, k) })
		if st.caretSet {
			in.SetTextSelection(st.newCaret, st.newCaret)
			st.caret, st.caretSet = st.newCaret, false
		} else {
			_, st.caret = in.TextSelection()
		}
		st.lastInput = st.input
	})
}

// aiKey takes the composer's Enter: alone it sends the question, and with
// Ctrl it breaks the line where the caret is, as a text area does for
// Shift+Enter. Anything else goes on as it would.
func (a *App) aiKey(s *Session, k ui.TextKey) bool {
	st := &s.ai
	if k.Key != ui.KeyEnter {
		return false
	}
	switch k.Mods {
	case 0:
		text := strings.TrimSpace(k.Text())
		if text == "" || st.busy {
			return true
		}
		k.SetText("")
		a.aiAsk(s, text)
		return true
	case ui.Ctrl:
		text := []rune(k.Text())
		at := st.caretAt(len(text))
		line := string(text[:at]) + "\n" + string(text[at:])
		k.SetText(line)
		st.newCaret, st.caretSet = at+1, true
		return true
	}
	return false
}

// caretAt is where the caret is in text as a key comes: where the last
// frame had it, moved by what was typed since.
func (st *aiChat) caretAt(n int) int {
	c := st.caret
	if last := len([]rune(st.lastInput)); last != n {
		c += n - last
	}
	return min(max(c, 0), n)
}

// openAISettings opens the options on the AI page.
func (a *App) openAISettings() {
	a.openOptions()
	a.optionsNav.Page = pageAI
}

// aiInsert writes text in s's terminal, for the user to run it.
func (a *App) aiInsert(s *Session, text string) {
	if s == nil || s.term() == nil || text == "" {
		return
	}
	a.sendLine(s, text, false)
}
