package app

import (
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The guard of the danger rules: the Enter that would run a command the rules
// catch is held until the user says yes, as the original holds it. A high
// rule's command asks for the rule's name to be typed out, a hint's for one
// confirmation. The keys typed so far have already gone to the shell, which
// is how a terminal works, so cancelling clears the line there with Ctrl+U
// rather than leaving it at the prompt for the next Enter.

// dangerGuardState is the command the guard holds: what it says, and how the
// two answers reach the shell.
type dangerGuardState struct {
	line string
	rule config.DangerRule
	// send runs the command the guard held: the Enter it took, and the count
	// for the assets window. clear takes the line back off the shell.
	send, clear func()
	// typed is what the user has typed of the rule's name, for a high rule.
	typed string
}

// dangerGuardWindow is the guard's window: modal to the main window, so that
// nothing goes on under it while the answer is pending.
var dangerGuardWindow = WindowSpec{
	Kind:      WindowDangerGuard,
	Title:     locales.DangerGuardTitle,
	Width:     520,
	Height:    300,
	MinWidth:  440,
	MinHeight: 280,
	Modal:     true,
}

// dangerKey holds the Enter that would run a command the rules catch, and
// reports whether it took the key.
func (a *App) dangerKey(s *Session, p *pane, mods ui.Modifiers, key ui.Key) bool {
	if mods != 0 || key != ui.KeyEnter || !a.pluginOn(pluginDanger) {
		return false
	}
	line := strings.TrimSpace(s.cmdLine)
	if line == "" {
		return false
	}
	m := a.dangerHit(line, s.cmdEdited)
	if m == nil {
		return false
	}
	// The line is the guard's now: the next Enter starts another one.
	s.cmdLine, s.cmdEdited = "", false
	a.askDanger(p, line, m)
	return true
}

// askDanger opens the guard's window on the command, which runs only if the
// user says yes.
func (a *App) askDanger(p *pane, line string, m *dangerMatch) {
	a.danger.blocked++
	a.guard = dangerGuardState{
		line: line,
		rule: m.rule,
		send: func() {
			a.assetCommand(line)
			if p != nil && p.term != nil {
				p.term.Send([]byte("\r"))
			}
		},
		clear: func() {
			if p != nil && p.term != nil {
				p.term.Send([]byte{0x15}) // Ctrl+U, which empties the line
			}
		},
	}
	spec := dangerGuardWindow
	// Closing the window in any way, Escape and the title bar's button
	// included, is a no: the line is cleared, never run.
	spec.Closed = func() {
		g := a.guard
		a.guard = dangerGuardState{}
		if g.clear != nil {
			g.clear()
		}
	}
	a.openWindow(spec, a.dangerGuardView)
}

// approveDanger runs the command the guard held.
func (a *App) approveDanger() {
	g := a.guard
	a.guard.clear = nil // it runs: nothing to clear
	a.closeWindow(WindowDangerGuard)
	if g.send != nil {
		g.send()
	}
}

// rejectDanger takes the held line back off the shell and runs nothing.
func (a *App) rejectDanger() { a.closeWindow(WindowDangerGuard) }

// dangerGuardView is the guard's window: the rule, why it caught the command,
// the command itself, and for a high rule the name to type out.
func (a *App) dangerGuardView(c *ui.Context) {
	r := a.guard.rule
	high := dangerHigh(r)
	color := assetRed
	if !high {
		color = assetOrange
	}
	name := dangerName(r)
	ui.Column(c).Fill().Padding(16, 18, 16, 18).Gap(10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, name).FontSize(14).FontWeight(700).TextColor(color).SingleLine().MinWidth(0)
			ui.Spacer(c)
			widgets.Muted(c, locales.DangerGuardBlocked(a.danger.blocked)).FontSize(10)
		})
		if note := dangerNote(r); note != "" {
			ui.Text(c, note).FontSize(11).TextColor(theme.TextMuted).MaxLines(2)
		}
		ui.Box(c).FillWidth().Padding(8, 10, 8, 10).Radius(4).
			Background(theme.Text.Alpha(0.04)).Border(1, theme.Border).Children(func() {
			ui.Text(c, a.guard.line).Font("monospace").FontSize(11).Selectable().MaxLines(3).
				Tooltip(a.guard.line)
		})
		if high {
			ui.Text(c, locales.DangerGuardType(name)).FontSize(11).TextColor(theme.TextMuted).MaxLines(2)
			ui.TextInput(c, &a.guard.typed).Font("monospace").Label(locales.DangerGuardType(name)).FillWidth()
		}
		ui.Spacer(c)
		ui.Row(c).Gap(8).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, locales.DangerGuardCancel()).Clicked() {
				a.rejectDanger()
			}
			label := locales.DangerGuardRunAnyway()
			if high {
				label = locales.DangerGuardRun()
			}
			ready := !high || strings.TrimSpace(a.guard.typed) == name
			if ui.PrimaryButton(c, label).Disabled(!ready).Clicked() {
				a.approveDanger()
			}
		})
	})
}
