package app

import (
	"strconv"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// telnetPage is the editor's page of a Telnet server: where it is. The
// login is the terminal's, once connected, as the server asks for it.
func (a *App) telnetPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	ui.Form(c, func() {
		ui.Fieldset(c, locales.SectionGeneral(), func() {
			ui.Field(c, locales.FieldName(), func() {
				input(ui.TextInput(c, &ed.name).Placeholder(locales.FieldNamePlaceholder()))
			})
			ui.Field(c, locales.FieldHost(), func() {
				input(ui.TextInput(c, &ed.addr).Placeholder(locales.FieldHostPlaceholder()).AutoFocus())
			}).Error(errs[fieldAddr])
			ui.Field(c, locales.FieldPort(), func() {
				input(ui.TextInput(c, &ed.port).Width(90))
			}).Error(errs[fieldPort]).Description(locales.TelnetLoginHint())
			ui.Field(c, locales.FieldNotes(), func() {
				ui.TextArea(c, &ed.notes).Lines(2, 4)
			})
		})
	})
}

// telnetProblems is what is wrong with the Telnet page: the address, and
// the port it is reached at.
func (ed *connEditor) telnetProblems(errs map[editorField]string) {
	if strings.TrimSpace(ed.addr) == "" {
		errs[fieldAddr] = locales.ErrHostRequired()
	}
	if port, err := strconv.Atoi(strings.TrimSpace(ed.port)); err != nil || port < 1 || port > 65535 {
		errs[fieldPort] = locales.ErrPortInvalid()
	}
}

// saveTelnet saves the Telnet page into h: its protocol and its address.
func (a *App) saveTelnet(h *config.Host, ed *connEditor) {
	h.Protocol = config.ProtocolTelnet
	h.Redis, h.Object, h.Serial, h.ADB = nil, nil, nil, nil
	h.Name, h.Notes = strings.TrimSpace(ed.name), ed.notes
	h.Addr = strings.TrimSpace(ed.addr)
	h.Port, _ = strconv.Atoi(strings.TrimSpace(ed.port))
	h.User, h.Auth, h.KeyPath = "", config.AuthPassword, ""
	h.NoMonitor = true
	h.Proxy, h.Tunnels = nil, nil
}
