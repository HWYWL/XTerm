package app

import (
	"fmt"
	"regexp"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// highlightColors are the colors a rule may take, as QuickTerm's presets
// use them.
var highlightColors = []string{
	"#FF5555", "#E5C07B", "#3FB950", "#8B949E", "#56D4DD", "#BC8CFF",
	"#58A6FF", "#A5D6FF", "#F0883E", "#F778BA", "#7EE787", "#79C0FF",
}

// highlightPage is the Text highlighting page of the terminal's options:
// the presets with their switches, then the user's rules, which the editor
// below adds and changes.
func (a *App) highlightPage(c *ui.Context) {
	widgets.Muted(c, locales.HighlightIntro()).FontSize(12)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Spacer(c)
		if ui.Button(c, a.highlightAllLabel()).Clicked() {
			a.toggleAllHighlight()
		}
	})

	ui.Text(c, locales.HighlightPresets()).Bold()
	for i := range a.highlight {
		if a.highlight[i].Builtin {
			a.highlightCard(c, i)
		}
	}

	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.HighlightCustom()).Bold().Grow(1)
		if ui.Button(c, locales.HighlightAdd()).Clicked() {
			a.startHighlightRule("")
		}
	})
	custom := 0
	for i := range a.highlight {
		if !a.highlight[i].Builtin {
			custom++
			a.highlightCard(c, i)
		}
	}
	if custom == 0 {
		widgets.Muted(c, locales.HighlightNoCustom()).FontSize(12)
	}
	if a.hlOpen {
		a.highlightEditor(c)
	}
}

// highlightCard is a rule: its color, its name over its pattern, and its
// switch; a custom one has the way into the editor.
func (a *App) highlightCard(c *ui.Context, i int) {
	r := &a.highlight[i]
	ui.Row(c).Key(r.ID).Gap(8).AlignItems(ui.Center).Padding(5, 8).Radius(6).
		Border(1, theme.Border).Children(func() {
		ui.Box(c).Size(8, 8).Radius(4).Background(highlightColor(r.Color)).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, highlightName(*r)).FontSize(12).SingleLine()
			ui.Text(c, r.Pattern).FontSize(11).TextColor(theme.TextMuted).SingleLine().Ellipsis("…")
		})
		if !r.Builtin {
			if ui.Button(c, locales.Edit()).Shrink(0).Clicked() {
				a.startHighlightRule(r.ID)
			}
		}
		on := r.Enabled
		ui.Switch(c, &on).Shrink(0).Label(highlightName(*r))
		if on != r.Enabled {
			r.Enabled = on
			a.saveHighlight()
		}
	})
}

// highlightEditor is the form of a custom rule, open below the list while a
// rule is being added or changed.
func (a *App) highlightEditor(c *ui.Context) {
	d := &a.hlDraft
	invalid := highlightExpr(*d) == nil
	ui.Column(c).Gap(8).Padding(10).Radius(8).Border(1, theme.Border).Children(func() {
		ui.Text(c, a.highlightEditorTitle()).Bold()
		ui.TextInput(c, &d.Name).Label(locales.HighlightName()).Width(320)
		ui.TextInput(c, &d.Pattern).Label(locales.HighlightPattern())
		ui.Checkbox(c, &d.IgnoreCase, locales.HighlightIgnoreCase())
		ui.Text(c, locales.HighlightColor()).FontSize(12)
		ui.Row(c).Gap(6).Children(func() {
			for _, col := range highlightColors {
				sw := ui.Box(c).Size(18, 18).Radius(4).Background(highlightColor(col)).
					Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(col)
				if d.Color == col {
					sw.Border(2, theme.Text)
				}
				if sw.Clicked() {
					d.Color = col
				}
			}
		})
		if invalid {
			widgets.Muted(c, locales.HighlightInvalid()).FontSize(12)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if !a.hlNew && ui.Button(c, locales.Delete()).Clicked() {
				a.deleteHighlightRule(d.ID)
				return
			}
			if ui.Button(c, locales.Cancel()).Clicked() {
				a.hlOpen = false
			}
			save := ui.PrimaryButton(c, locales.Save()).Disabled(invalid)
			if !invalid && save.Clicked() {
				a.saveHighlightDraft()
			}
		})
	})
}

// startHighlightRule opens the editor on the rule of id, or on a new one
// when id is empty.
func (a *App) startHighlightRule(id string) {
	a.hlNew = id == ""
	a.hlDraft = config.HighlightRule{
		ID:      fmt.Sprintf("hl-custom-%d", time.Now().UnixNano()),
		Color:   highlightColors[0],
		Enabled: true,
	}
	if !a.hlNew {
		for _, r := range a.highlight {
			if r.ID == id {
				a.hlDraft = r
				break
			}
		}
	}
	a.hlOpen = true
}

// saveHighlightDraft puts the editor's rule in the list, and keeps it.
func (a *App) saveHighlightDraft() {
	draft := a.hlDraft
	draft.Builtin = false
	if a.hlNew {
		a.highlight = append(a.highlight, draft)
	} else {
		for i := range a.highlight {
			if a.highlight[i].ID == draft.ID {
				a.highlight[i] = draft
			}
		}
	}
	a.hlOpen = false
	a.saveHighlight()
}

// deleteHighlightRule drops the rule of id.
func (a *App) deleteHighlightRule(id string) {
	kept := a.highlight[:0]
	for _, r := range a.highlight {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	a.highlight = kept
	a.hlOpen = false
	a.saveHighlight()
}

// highlightAllLabel is what the button over the list says: the way to the
// opposite of where the rules are, as QuickTerm's one button does.
func (a *App) highlightAllLabel() string {
	for _, r := range a.highlight {
		if !r.Enabled {
			return locales.HighlightAllOn()
		}
	}
	return locales.HighlightAllOff()
}

// toggleAllHighlight turns every rule on, or all of them off.
func (a *App) toggleAllHighlight() {
	on := true
	for _, r := range a.highlight {
		if !r.Enabled {
			on = false
			break
		}
	}
	on = !on
	for i := range a.highlight {
		a.highlight[i].Enabled = on
	}
	a.saveHighlight()
}

// highlightEditorTitle is what the editor is about.
func (a *App) highlightEditorTitle() string {
	if a.hlNew {
		return locales.HighlightNewRule()
	}
	return locales.HighlightEditRule()
}

// highlightExpr compiles a rule's expression, error and all, for the
// editor to tell the user about a pattern that does not compile.
func highlightExpr(r config.HighlightRule) error {
	if r.Pattern == "" {
		return nil
	}
	_, err := regexp.Compile(highlightPattern(r))
	return err
}
