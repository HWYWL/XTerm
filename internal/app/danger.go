package app

import (
	"regexp"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The danger rules window: the patterns a command is judged against before it
// runs, as the original's list has them: each rule named, explained, shown as
// the regular expression it is, tagged high or hint, and switched on or off
// one at a time. The rules are judged in the order the window lists them, the
// first that matches saying what kind of command it is (dangerguard.go holds
// the Enter it takes), and the assets window counts the commands they catch.

// dangerText is a rule the app ships: its name and note, in the language
// shown.
type dangerText struct {
	name, note func() string
}

// dangerTexts are the texts of the rules the app ships, by their ID.
var dangerTexts = map[string]dangerText{
	"rm-root":         {locales.DangerRuleRmRoot, locales.DangerRuleRmRootNote},
	"rm-home":         {locales.DangerRuleRmHome, locales.DangerRuleRmHomeNote},
	"rm-cwd":          {locales.DangerRuleRmCwd, locales.DangerRuleRmCwdNote},
	"disk-wipe":       {locales.DangerRuleDiskWipe, locales.DangerRuleDiskWipeNote},
	"fork-bomb":       {locales.DangerRuleFork, locales.DangerRuleForkNote},
	"overwrite-etc":   {locales.DangerRuleOverwrite, locales.DangerRuleOverwriteNote},
	"shutdown":        {locales.DangerRuleShutdown, locales.DangerRuleShutdownNote},
	"chmod-root":      {locales.DangerRuleChmod, locales.DangerRuleChmodNote},
	"drop-db":         {locales.DangerRuleDropDb, locales.DangerRuleDropDbNote},
	"redis-flush":     {locales.DangerRuleRedisFlush, locales.DangerRuleRedisFlushNote},
	"container-prune": {locales.DangerRuleContainerPrune, locales.DangerRuleContainerPruneNote},
	"git-force":       {locales.DangerRuleGitForce, locales.DangerRuleGitForceNote},
	"curl-pipe-sh":    {locales.DangerRuleCurlPipeSh, locales.DangerRuleCurlPipeShNote},
	"firewall-off":    {locales.DangerRuleFirewallOff, locales.DangerRuleFirewallOffNote},
}

// dangerMatch is a rule that switched on, with its pattern compiled: what a
// command that matches it is answered with.
type dangerMatch struct {
	rule config.DangerRule
	re   *regexp.Regexp
}

// dangerState is the rules, the ones switched on compiled for every command
// typed, the draft of a new rule, what adding failed with, and the commands
// the guard has held this run.
type dangerState struct {
	rules   []config.DangerRule
	hits    []dangerMatch
	inexact dangerMatch // a line the arrows went through; see dangerHit
	draft   string
	err     string
	blocked int
}

// loadDanger reads the rules and compiles them.
func (d *dangerState) load() {
	d.rules = config.LoadDangerRules()
	d.inexact = dangerMatch{rule: config.DangerRule{
		Name:  locales.DangerGuardInexact(),
		Note:  locales.DangerGuardInexactNote(),
		Level: config.DangerHint,
	}}
	d.compile()
}

// compile takes the patterns that are regular expressions and switched on:
// one hand-edited into a file that is not is left to the window to show, and
// matches nothing.
func (d *dangerState) compile() {
	d.hits = d.hits[:0]
	for _, r := range d.rules {
		if r.Off {
			continue
		}
		if re, err := regexp.Compile(r.Pattern); err == nil {
			d.hits = append(d.hits, dangerMatch{rule: r, re: re})
		}
	}
}

// destructiveVerb and destructiveTarget are how a line that the app could not
// follow whole is judged: both have to be there, as a line that only says `rm`
// or only has a slash is no command to ask about.
var (
	destructiveVerb   = regexp.MustCompile(`(?i)\b(rm|dd|mkfs|shred|truncate|drop|flushall|shutdown|reboot)\b`)
	destructiveTarget = regexp.MustCompile(`[/*~]|\$HOME`)
)

// dangerHit is the rule the line matches, nil for one nothing matches: the
// rules switched on in order, the first that matches the one reported. A line
// the arrows went through may not be what the shell has, so one that still
// looks destructive is asked about once, as a hint.
func (a *App) dangerHit(line string, edited bool) *dangerMatch {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	for i := range a.danger.hits {
		if a.danger.hits[i].re.MatchString(line) {
			return &a.danger.hits[i]
		}
	}
	if edited && destructiveVerb.MatchString(line) && destructiveTarget.MatchString(line) {
		return &a.danger.inexact
	}
	return nil
}

// risky tells whether line matches one of the rules switched on, for the
// assets window.
func (a *App) risky(line string) bool { return a.dangerHit(line, false) != nil }

// dangerName is the rule's name: the app's text for one of its own, the
// user's for one they added, the pattern for one that has neither.
func dangerName(r config.DangerRule) string {
	if t, ok := dangerTexts[r.ID]; ok && r.Name == "" {
		return t.name()
	}
	if r.Name != "" {
		return r.Name
	}
	return r.Pattern
}

// dangerNote is why the rule's commands are dangerous: the app's text for one
// of its own, the user's for one they added.
func dangerNote(r config.DangerRule) string {
	if t, ok := dangerTexts[r.ID]; ok && r.Note == "" {
		return t.note()
	}
	return r.Note
}

// dangerHigh tells whether the rule marks a command that loses data, rather
// than one that only interrupts what runs.
func dangerHigh(r config.DangerRule) bool { return r.Level != config.DangerHint }

// keepDanger saves the rules, and compiles them for the next command.
func (a *App) keepDanger() {
	if err := config.SaveDangerRules(a.danger.rules); err != nil {
		a.danger.err = err.Error()
	}
	a.danger.compile()
}

// addDangerRule adds the draft, which must be a regular expression.
func (a *App) addDangerRule() {
	pattern := strings.TrimSpace(a.danger.draft)
	if pattern == "" {
		return
	}
	if _, err := regexp.Compile(pattern); err != nil {
		a.danger.err = locales.DangerInvalid()
		return
	}
	a.danger.rules = append(a.danger.rules, config.DangerRule{Pattern: pattern, Level: config.DangerHigh})
	a.danger.draft, a.danger.err = "", ""
	a.keepDanger()
}

// removeDangerRule removes the rule at i.
func (a *App) removeDangerRule(i int) {
	if i < 0 || i >= len(a.danger.rules) {
		return
	}
	a.danger.rules = append(a.danger.rules[:i], a.danger.rules[i+1:]...)
	a.danger.err = ""
	a.keepDanger()
}

// setDangerOff switches the rule at i off, or on again.
func (a *App) setDangerOff(i int, off bool) {
	if i < 0 || i >= len(a.danger.rules) || a.danger.rules[i].Off == off {
		return
	}
	a.danger.rules[i].Off = off
	a.danger.err = ""
	a.keepDanger()
}

// toggleDangerLevel makes the rule at i high where it was a hint, and a hint
// where it was high, and marks the level as the user's: a later set of the
// app's rules keeps it.
func (a *App) toggleDangerLevel(i int) {
	if i < 0 || i >= len(a.danger.rules) {
		return
	}
	if dangerHigh(a.danger.rules[i]) {
		a.danger.rules[i].Level = config.DangerHint
	} else {
		a.danger.rules[i].Level = config.DangerHigh
	}
	a.danger.rules[i].LevelCustom = true
	a.keepDanger()
}

// switchAllDanger switches every rule on, or every one off.
func (a *App) switchAllDanger(on bool) {
	for i := range a.danger.rules {
		a.danger.rules[i].Off = !on
	}
	a.danger.err = ""
	a.keepDanger()
}

// restoreDangerRules puts the app's rules back.
func (a *App) restoreDangerRules() {
	a.danger.rules = config.DefaultDangerRules()
	a.danger.draft, a.danger.err = "", ""
	a.keepDanger()
}

// dangerWindow is the rules window: a list, so wider than the plugin center.
var dangerWindow = WindowSpec{
	Kind:      WindowDangerRules,
	Title:     locales.DangerTitle,
	Width:     780,
	Height:    620,
	MinWidth:  620,
	MinHeight: 420,
}

// openDangerRules opens the rules window, or brings it to the front.
func (a *App) openDangerRules() { a.openWindow(dangerWindow, a.dangerView) }

// dangerView is the window: the note, the count and the switches for all the
// rules, the input that adds one, and the rules themselves.
func (a *App) dangerView(c *ui.Context) {
	on := 0
	for _, r := range a.danger.rules {
		if !r.Off {
			on++
		}
	}
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(10).Children(func() {
		ui.Text(c, locales.DangerNote()).FontSize(11).TextColor(theme.TextMuted).MaxLines(3)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			widgets.Muted(c, locales.DangerOn(on, len(a.danger.rules))).FontSize(11)
			ui.Spacer(c)
			all := locales.DangerAllOff()
			if on == 0 && len(a.danger.rules) > 0 {
				all = locales.DangerAllOn()
			}
			if ui.Button(c, all).FontSize(11).Disabled(len(a.danger.rules) == 0).Clicked() {
				a.switchAllDanger(on == 0)
			}
			if ui.Button(c, locales.DangerRestore()).FontSize(11).Clicked() {
				a.restoreDangerRules()
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			submitted := ui.TextInput(c, &a.danger.draft).Placeholder(locales.DangerPattern()).
				Font("monospace").Grow(1).MinWidth(0).Label(locales.DangerPattern()).Submitted()
			if ui.Button(c, locales.DangerAdd()).Disabled(strings.TrimSpace(a.danger.draft) == "").Clicked() || submitted {
				a.addDangerRule()
			}
		})
		if a.danger.err != "" {
			ui.Text(c, a.danger.err).FontSize(11).TextColor(theme.Danger)
		}
		// The guard asks for this note as much as the list does.
		if !a.pluginOn(pluginDanger) {
			widgets.Muted(c, locales.DangerGuardOff())
		}
		if len(a.danger.rules) == 0 {
			widgets.Muted(c, locales.DangerEmpty())
			return
		}
		// The rows are drawn from a copy: one of them may be deleted, or
		// moved, by the click it takes.
		rules := append([]config.DangerRule(nil), a.danger.rules...)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			for i, r := range rules {
				a.dangerRuleRow(c, i, r)
			}
		})
	})
}

// dangerRuleRow is one rule: its name and why it is dangerous, its pattern,
// the tag of its level and the switch that turns it off, over a rule that
// deletes it.
func (a *App) dangerRuleRow(c *ui.Context, i int, r config.DangerRule) {
	row := ui.Column(c).Key(i).Gap(6).Padding(8, 10, 8, 10).Radius(6).Border(1, theme.Border)
	if r.Off {
		row.Border(1, theme.Border.Alpha(0.5))
	}
	row.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
				name := ui.Text(c, dangerName(r)).FontSize(12).FontWeight(600).SingleLine().MinWidth(0)
				if r.Off {
					name.TextColor(theme.TextMuted)
				}
				if note := dangerNote(r); note != "" {
					ui.Text(c, note).FontSize(10).TextColor(theme.TextMuted).SingleLine().
						MinWidth(0).Ellipsis("…").Tooltip(note)
				}
			})
			a.dangerLevelPill(c, i, r)
			on := !r.Off
			ui.Switch(c, &on).Label(dangerName(r)).Shrink(0)
			if on != !r.Off {
				a.setDangerOff(i, !on)
			}
			if widgets.IconButton(c, "trash-2", locales.Delete()).Clicked() {
				a.removeDangerRule(i)
			}
		})
		ui.Box(c).FillWidth().Padding(6, 8, 6, 8).Radius(4).
			Background(theme.Text.Alpha(0.04)).Border(1, theme.Border).Children(func() {
			ui.Text(c, r.Pattern).Font("monospace").FontSize(10).SingleLine().MinWidth(0).
				Ellipsis("…").Tooltip(r.Pattern)
		})
		if _, err := regexp.Compile(r.Pattern); err != nil {
			ui.Text(c, locales.DangerInvalid()).FontSize(10).TextColor(theme.Danger)
		}
	})
}

// dangerLevelPill is the tag of the rule's level, which a click flips.
func (a *App) dangerLevelPill(c *ui.Context, i int, r config.DangerRule) {
	label, color := locales.DangerHigh(), assetRed
	if !dangerHigh(r) {
		label, color = locales.DangerHint(), assetOrange
	}
	pill := ui.Box(c).Key("level").Role(ui.RoleButton).Label(label).Shrink(0).
		Height(20).Padding(0, 8, 0, 8).Radius(10).
		Border(1, color.Alpha(0.45)).Background(color.Alpha(0.12))
	if pill.Hovered() {
		pill.Background(color.Alpha(0.2))
	}
	pill.Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(color).SingleLine()
	})
	if pill.Clicked() {
		a.toggleDangerLevel(i)
	}
}
