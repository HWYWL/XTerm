package app

import (
	"slices"
	"strconv"

	"finallyshell/internal/redis"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// redisSectionsDialog is INFO's sections as a dialog, opened from a Redis
// tab's System page: the sections to choose from at its top, and under
// them the fields of the chosen one — the field, its value, and what the
// field means, as redis.io documents it.
type redisSectionsDialog struct {
	s       *Session
	section string
}

// openRedisSections opens the dialog on a section of the session's INFO,
// one dialog at a time.
func (a *App) openRedisSections(s *Session, section string) {
	a.redisSectionsDlg = &redisSectionsDialog{s: s, section: section}
}

// redisSectionsDialogs draws the dialog while it is open, and closes it
// when its session goes.
func (a *App) redisSectionsDialogs(c *ui.Context) {
	d := a.redisSectionsDlg
	if d == nil {
		return
	}
	open := true
	ui.Modal(c, &open, func() { a.redisSectionsView(c, d, &open) })
	if !open || d.s.status != statusConnected || !slices.Contains(a.sessions, d.s) {
		if a.redisSectionsDlg == d {
			a.redisSectionsDlg = nil
		}
	}
}

// redisSectionsView is the dialog: the sections, the chosen one in the
// accent color, and the table of its fields.
func (a *App) redisSectionsView(c *ui.Context, d *redisSectionsDialog, open *bool) {
	in := d.s.redisInfo
	if len(in.Sections) == 0 {
		ui.Column(c).Width(420).Gap(10).Children(func() {
			widgets.Muted(c, locales.ReadingServerInfo())
			if ui.Button(c, locales.Close()).Clicked() {
				*open = false
			}
		})
		return
	}
	if in.Section(d.section) == nil {
		d.section = in.Sections[0].Name
	}
	sec := in.Section(d.section)
	addr := d.s.redisOpts.Addr
	if addr == "" && d.s.Host != nil {
		addr = d.s.Host.Target()
	}
	ui.Column(c).Width(780).MaxWidth(920).Children(func() {
		ui.Row(c).Padding(12, 16).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.Box(c).Size(3, 18).Radius(2).Background(theme.Accent).Shrink(0)
			ui.Text(c, locales.RedisSectionsTitle()).FontSize(15).Bold().Shrink(0)
			widgets.Muted(c, addr).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			if ui.Button(c, locales.Close()).Clicked() {
				*open = false
			}
		})
		ui.Scroll(c).MaxHeight(560).Children(func() {
			ui.Column(c).Padding(14, 16, 16, 16).Gap(12).Children(func() {
				a.redisSectionPills(c, d, in)
				ui.Column(c).FillWidth().Children(func() {
					ui.Row(c).Padding(6, 8).Gap(10).AlignItems(ui.Center).
						BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
						widgets.Muted(c, locales.RedisColField()).Width(200).Shrink(0).FontSize(11)
						widgets.Muted(c, locales.RedisColValue()).Width(220).Shrink(0).FontSize(11)
						widgets.Muted(c, locales.RedisColMeaning()).Grow(1).FontSize(11)
						widgets.Muted(c, strconv.Itoa(len(sec.Fields))).Shrink(0).FontSize(11)
					})
					for _, f := range sec.Fields {
						meaning := redisInfoDoc(f.Key)
						shown := meaning
						if shown == "" { // a field of a newer server than the app knows
							shown = locales.RedisNoMeaning()
						}
						ui.Row(c).Padding(5, 8).Gap(10).AlignItems(ui.Center).
							BorderWidth(0, 0, 1, 0).BorderColor(colors.rowLine).Children(func() {
							ui.Text(c, f.Key).Width(200).Shrink(0).FontSize(11.5).SingleLine().Tooltip(f.Key)
							ui.Text(c, f.Value).Width(220).Shrink(0).FontSize(11.5).SingleLine().
								Selectable().Tooltip(f.Value)
							widgets.Muted(c, shown).Grow(1).MinWidth(0).FontSize(11).SingleLine().
								Tooltip(shown)
						})
					}
				})
			})
		})
	})
}

// redisSectionPills is the sections of INFO as pills, the chosen one in
// the accent color; one chosen stays chosen as the dialog, and the page
// behind it, shows it.
func (a *App) redisSectionPills(c *ui.Context, d *redisSectionsDialog, in redis.Info) {
	ui.Row(c).Gap(6).Wrap().Children(func() {
		for _, sec := range in.Sections {
			chosen := sec.Name == d.section
			pill := ui.Row(c).Height(24).Padding(0, 11).Radius(12).Border(1, theme.Border).
				AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
			switch {
			case chosen:
				pill.Background(theme.Accent)
			case pill.Hovered():
				pill.Background(theme.SurfaceHover)
			}
			pill.Children(func() {
				t := ui.Text(c, sec.Name).FontSize(11.5).SingleLine()
				if chosen {
					t.TextColor(theme.AccentText).FontWeight(600)
				} else {
					t.TextColor(theme.TextMuted)
				}
			})
			if pill.Clicked() {
				d.section, a.redisSection = sec.Name, sec.Name
			}
		}
	})
}
