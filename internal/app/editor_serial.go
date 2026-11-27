package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// serialBauds are the speeds the editor offers: the ones consoles and
// boards use, from the slowest to the fastest.
var serialBauds = []string{"1200", "2400", "4800", "9600", "19200", "38400", "57600", "115200", "230400", "460800", "921600"}

// serialParities are the parity settings, as the editor names them and as
// the port takes them.
func serialParities() (names, values []string) {
	return []string{
			locales.SerialParityNone(), locales.SerialParityEven(), locales.SerialParityOdd(),
			locales.SerialParityMark(), locales.SerialParitySpace(),
		},
		[]string{"none", "even", "odd", "mark", "space"}
}

// serialPage is the editor's page of a device on a serial port: where the
// port is, and how the line is set.
func (a *App) serialPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	ui.Form(c, func() {
		ui.Fieldset(c, locales.SectionGeneral(), func() {
			ui.Field(c, locales.FieldName(), func() {
				input(ui.TextInput(c, &ed.name).Placeholder(locales.FieldNamePlaceholder()))
			})
			ui.Field(c, locales.FieldSerialPort(), func() {
				// The ports this computer has, as a list; a port the list
				// does not have is typed, from the last item.
				if len(ed.serialPorts) == 0 || ed.serialPortCustom {
					input(ui.TextInput(c, &ed.serialPort).Placeholder(locales.FieldSerialPortHint()).AutoFocus())
					return
				}
				names := append(slices.Clone(ed.serialPorts), locales.FieldSerialPortCustom())
				chosen := ed.serialPort
				if chosen == "" {
					chosen = ed.serialPorts[0]
					ed.serialPort = chosen
				}
				if ui.Select(c, &chosen, names).Width(220).Changed() {
					if chosen == locales.FieldSerialPortCustom() {
						ed.serialPortCustom, ed.serialPort = true, ""
						return
					}
					ed.serialPort = chosen
				}
			}).Error(errs[fieldSerialPort]).Description(serialPortHint(ed.serialPorts))
			ui.Field(c, locales.FieldSerialBaud(), func() {
				ui.Select(c, &ed.serialBaud, serialBauds).Width(140)
			})
			ui.Field(c, locales.FieldSerialDataBits(), func() {
				ui.Select(c, &ed.serialDataBits, []string{"5", "6", "7", "8"}).Width(90)
			})
			ui.Field(c, locales.FieldSerialParity(), func() {
				names, values := serialParities()
				chosen := names[slices.Index(values, ed.serialParity)]
				if ui.Select(c, &chosen, names).Width(140).Changed() {
					ed.serialParity = values[slices.Index(names, chosen)]
				}
			})
			ui.Field(c, locales.FieldSerialStopBits(), func() {
				ui.Select(c, &ed.serialStopBits, []string{"1", "1.5", "2"}).Width(90)
			})
			ui.Field(c, locales.FieldNotes(), func() {
				ui.TextArea(c, &ed.notes).Lines(2, 4)
			})
		})
	})
}

// hasSerialPort tells whether the list of the computer's ports has port.
func hasSerialPort(ports []string, port string) bool { return slices.Contains(ports, port) }

// serialPortHint is what the port field says under it: how a port is
// named, and which ones this computer has.
func serialPortHint(ports []string) string {
	if len(ports) == 0 {
		return locales.FieldSerialPortHint()
	}
	return locales.FieldSerialPortHint() + " · " + fmt.Sprintf(locales.FieldSerialPorts(), strings.Join(ports, ", "))
}

// serialProblems is what is wrong with the serial page: the port is the
// one thing a serial host cannot do without.
func (ed *connEditor) serialProblems(errs map[editorField]string) {
	if strings.TrimSpace(ed.serialPort) == "" {
		errs[fieldSerialPort] = locales.ErrSerialPortRequired()
	}
}

// saveSerial saves the serial page into h: its protocol, the port and the
// line's settings.
func (a *App) saveSerial(h *config.Host, ed *connEditor) {
	h.Protocol = config.ProtocolSerial
	h.Redis, h.Object = nil, nil
	if h.Serial == nil {
		h.Serial = &config.SerialSettings{}
	}
	h.Name, h.Notes = strings.TrimSpace(ed.name), ed.notes
	h.Addr, h.Port, h.User, h.Auth, h.KeyPath = "", 0, "", config.AuthPassword, ""
	h.NoMonitor = true
	h.TermType, h.Backspace, h.Delete, h.Encoding = "", "", "", ""
	h.Proxy, h.Tunnels = nil, nil
	h.Serial.Port = strings.TrimSpace(ed.serialPort)
	h.Serial.Baud, _ = strconv.Atoi(ed.serialBaud)
	h.Serial.DataBits, _ = strconv.Atoi(ed.serialDataBits)
	h.Serial.Parity, h.Serial.StopBits = ed.serialParity, ed.serialStopBits
}
