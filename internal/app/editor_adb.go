package app

import (
	"slices"
	"strconv"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// adbModes are the ways a device is reached, as the editor names them and
// as the host keeps them.
func adbModes() (names, values []string) {
	return []string{locales.AdbWired(), locales.AdbWireless()},
		[]string{config.ADBUS, config.ADBTCP}
}

// adbPage is the editor's page of an Android device: whether it is on a
// cable or the network, and where it is.
func (a *App) adbPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	names, values := adbModes()
	mode := ed.adbModeOrDefault()
	chosen := names[slices.Index(values, mode)]
	ui.Form(c, func() {
		ui.Fieldset(c, locales.SectionGeneral(), func() {
			ui.Field(c, locales.FieldName(), func() {
				input(ui.TextInput(c, &ed.name).Placeholder(locales.FieldNamePlaceholder()))
			})
			ui.Field(c, locales.FieldADBConnection(), func() {
				if ui.Select(c, &chosen, names).Width(200).Changed() {
					ed.adbMode = values[slices.Index(names, chosen)]
					if ed.adbMode == config.ADBTCP && strings.TrimSpace(ed.port) == "22" {
						ed.port = strconv.Itoa(config.ADBPort)
					}
				}
			})
			if mode == config.ADBTCP {
				ui.Field(c, locales.FieldHost(), func() {
					input(ui.TextInput(c, &ed.addr).Placeholder(locales.FieldHostPlaceholder()).AutoFocus())
				}).Error(errs[fieldAddr])
				ui.Field(c, locales.FieldPort(), func() {
					ui.Row(c).Children(func() {
						input(ui.TextInput(c, &ed.port).Width(90))
					})
				}).Error(errs[fieldPort]).Description(locales.AdbHint())
			} else {
				ui.Field(c, locales.FieldADBDevice(), func() {
					ui.Row(c).Children(func() {
						input(ui.TextInput(c, &ed.adbDevice).Placeholder(locales.FieldADBDeviceHint()).AutoFocus())
					})
				}).Description(locales.AdbWiredHint())
				ui.Field(c, locales.AdbTools(), func() {
					a.adbToolsRow(c)
				})
			}
			ui.Field(c, locales.FieldNotes(), func() {
				ui.TextArea(c, &ed.notes).Lines(2, 4)
			})
		})
		if mode == config.ADBTCP {
			ui.Fieldset(c, locales.AdbPair(), func() {
				ui.Field(c, locales.AdbPairPort(), func() {
					ui.Row(c).Children(func() {
						ui.TextInput(c, &ed.adbPairPort).Placeholder(locales.AdbPairHint())
					})
				})
				ui.Field(c, locales.AdbPairCode(), func() {
					ui.Row(c).Children(func() {
						ui.TextInput(c, &ed.adbPairCode).Placeholder("123456").Width(120)
					})
				})
				ui.Field(c, "", func() {
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						pair := ui.Button(c, locales.AdbPairButton()).
							Disabled(ed.adbPairing || strings.TrimSpace(ed.adbPairPort) == "" ||
								strings.TrimSpace(ed.adbPairCode) == "")
						if pair.Clicked() {
							a.adbPair(ed)
						}
						if ed.adbPairing {
							widgets.Muted(c, locales.AdbPairingNow()).FontSize(11)
						}
					})
				})
				if ed.adbPairMsg != "" {
					color := theme.TextMuted
					if ed.adbPairErr {
						color = theme.Danger
					}
					ui.Text(c, ed.adbPairMsg).FontSize(11).TextColor(color).Wrap()
				}
			})
		}
	})
}

// adbModeOrDefault is the way the host is reached, a cable unless the
// editor says otherwise.
func (ed *connEditor) adbModeOrDefault() string {
	if ed.adbMode == config.ADBTCP {
		return config.ADBTCP
	}
	return config.ADBUS
}

// adbProblems is what is wrong with the ADB page: a device on the network
// needs its address, and the port it listens on.
func (ed *connEditor) adbProblems(errs map[editorField]string) {
	if ed.adbModeOrDefault() != config.ADBTCP {
		return
	}
	if strings.TrimSpace(ed.addr) == "" {
		errs[fieldAddr] = locales.ErrHostRequired()
	}
	if port, err := strconv.Atoi(strings.TrimSpace(ed.port)); err != nil || port < 1 || port > 65535 {
		errs[fieldPort] = locales.ErrPortInvalid()
	}
}

// saveADB saves the ADB page into h: its protocol, how the device is
// reached, and where.
func (a *App) saveADB(h *config.Host, ed *connEditor) {
	mode := ed.adbModeOrDefault()
	h.Protocol = config.ProtocolADB
	h.Redis, h.Object, h.Serial = nil, nil, nil
	h.Name, h.Notes = strings.TrimSpace(ed.name), ed.notes
	h.User, h.Auth, h.KeyPath = "", config.AuthPassword, ""
	h.NoMonitor = true
	h.TermType, h.Backspace, h.Delete, h.Encoding = "", "", "", ""
	h.Proxy, h.Tunnels = nil, nil
	h.ADB = &config.ADBSettings{Mode: mode, Device: strings.TrimSpace(ed.adbDevice)}
	if mode == config.ADBTCP {
		h.Addr = strings.TrimSpace(ed.addr)
		h.Port, _ = strconv.Atoi(strings.TrimSpace(ed.port))
	} else {
		h.Addr, h.Port = "", 0
	}
}
