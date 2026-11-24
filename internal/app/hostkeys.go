package app

import (
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The host keys window: what sshx trusts in known_hosts, to look at and to
// forget, as QuickTerm's host keys plugin does. Forgetting a key makes the
// next connection to that server ask for it again: the way out of a key that
// changed after a reinstall, when the app refuses to connect.

// hostKeysState is the window's state: the entries read when it opened, the
// line whose deletion a click asked for, and what reading failed with.
type hostKeysState struct {
	entries []config.KnownHostEntry
	pending int
	err     string
}

// loadHostKeys reads the file again, as it opened or after a deletion.
func (a *App) loadHostKeys() {
	entries, err := config.LoadKnownHosts()
	a.hostKeys.entries, a.hostKeys.pending, a.hostKeys.err = entries, 0, ""
	if err != nil {
		a.hostKeys.err = err.Error()
	}
}

// hostKeysWindow is its spec: a list, so wider than the plugin center.
var hostKeysWindow = WindowSpec{
	Kind:      WindowHostKeys,
	Title:     locales.HostKeysTitle,
	Width:     780,
	Height:    560,
	MinWidth:  620,
	MinHeight: 400,
}

// openHostKeys opens the window, reading the file each time it opens.
func (a *App) openHostKeys() {
	if !a.windowOpen(WindowHostKeys) {
		a.loadHostKeys()
	}
	a.openWindow(hostKeysWindow, a.hostKeysView)
}

// hostKeysView is the window: the note, then the keys, one a row.
func (a *App) hostKeysView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(10).Children(func() {
		ui.Text(c, locales.HostKeysNote()).FontSize(11).TextColor(theme.TextMuted).MaxLines(3)
		if a.hostKeys.err != "" {
			ui.Text(c, a.hostKeys.err).FontSize(11).TextColor(theme.Danger).Selectable()
		}
		if len(a.hostKeys.entries) == 0 {
			widgets.Muted(c, locales.HostKeysEmpty())
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			for _, e := range a.hostKeys.entries {
				a.hostKeyRow(c, e)
			}
		})
	})
}

// hostKeyRow is one line: its hosts, its key and fingerprint, and the button
// that forgets it, which asks once more with a second click.
func (a *App) hostKeyRow(c *ui.Context, e config.KnownHostEntry) {
	row := ui.Row(c).Key(e.Line).Gap(10).AlignItems(ui.Center).
		Padding(6, 8, 6, 8).Radius(6).Border(1, theme.Border)
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
			hosts := strings.Join(e.Hosts, ", ")
			if e.Hashed() {
				hosts = locales.HostKeysHashed()
			}
			if e.Marker != "" {
				hosts = e.Marker + " " + hosts
			}
			ui.Text(c, hosts).FontSize(12).SingleLine().MinWidth(0)
			ui.Text(c, e.KeyType+"  "+e.Fingerprint()).FontSize(10).
				TextColor(theme.TextMuted).SingleLine().Ellipsis("…")
		})
		if a.hostKeys.pending == e.Line {
			if ui.PrimaryButton(c, locales.HostKeysConfirm()).Clicked() {
				a.forgetHostKey(e.Line)
			}
			if ui.Button(c, locales.Cancel()).Clicked() {
				a.hostKeys.pending = 0
			}
		} else if ui.Button(c, locales.HostKeysDelete()).Clicked() {
			a.hostKeys.pending = e.Line
		}
	})
}

// forgetHostKey deletes the line from known_hosts and reads the file again.
func (a *App) forgetHostKey(line int) {
	if err := config.DeleteKnownHosts(map[int]bool{line: true}); err != nil {
		a.hostKeys.err = err.Error()
		return
	}
	a.loadHostKeys()
}
