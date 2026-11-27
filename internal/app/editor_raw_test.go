package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// A serial host is saved with its port and the line's settings: the
// defaults are a console's, and the lists change them.
func TestEditorSavesSerialHost(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.ProtocolSerial()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	// The port is typed here, whatever the computer's own ports are.
	a.editor.serialPortCustom = true
	tt.Frame()
	if !tt.HasText(locales.EditorPageSerial()) {
		t.Fatalf("no serial page; texts %q", tt.Texts())
	}
	// The line starts as a console's, and the editor asks for no login.
	if a.editor.serialBaud != "115200" || a.editor.serialDataBits != "8" ||
		a.editor.serialParity != "none" || a.editor.serialStopBits != "1" {
		t.Errorf("defaults %q %q %q %q", a.editor.serialBaud, a.editor.serialDataBits,
			a.editor.serialParity, a.editor.serialStopBits)
	}
	if tt.HasText(locales.FieldUser()) || tt.HasText(locales.FieldPassword()) {
		t.Errorf("a serial page asks for a login; texts %q", tt.Texts())
	}
	typeInto(t, tt, locales.FieldName(), "console")
	typeInto(t, tt, locales.FieldSerialPort(), "COM4")
	choose(t, tt, locales.FieldSerialBaud(), "57600")
	choose(t, tt, locales.FieldSerialDataBits(), "7")
	choose(t, tt, locales.FieldSerialParity(), locales.SerialParityEven())
	choose(t, tt, locales.FieldSerialStopBits(), "2")
	saveShot(t, tt, "editor-serial-en.png")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if a.windowOpen(WindowEditor) {
		t.Fatalf("the editor is open: %q", tt.Texts())
	}
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	h := a.hosts.Hosts[0]
	if h.ProtocolOrDefault() != config.ProtocolSerial || !h.IsSerial() {
		t.Fatalf("host %+v", h)
	}
	if s := h.SerialPort(); s.Port != "COM4" || s.Baud != 57600 || s.DataBits != 7 ||
		s.Parity != "even" || s.StopBits != "2" {
		t.Errorf("serial %+v", s)
	}
	if h.Target() != "COM4" {
		t.Errorf("target %q", h.Target())
	}
	if h.User != "" || h.Addr != "" || h.Port != 0 || !h.NoMonitor {
		t.Errorf("host %+v", h)
	}
}

// The ports the computer has are a list, and a port it does not list is
// typed from the last item.
func TestEditorSerialPortList(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.ProtocolSerial()); err != nil {
		t.Fatal(err)
	}
	a.editor.serialPorts = []string{"COM3", "COM7"}
	a.editor.serialPortCustom, a.editor.serialPort = false, ""
	tt.Frame()
	choose(t, tt, locales.FieldSerialPort(), "COM7")
	if a.editor.serialPort != "COM7" {
		t.Fatalf("the port %q", a.editor.serialPort)
	}
	saveShot(t, tt, "editor-serial-list-en.png")
	choose(t, tt, locales.FieldSerialPort(), locales.FieldSerialPortCustom())
	tt.Frame()
	if !a.editor.serialPortCustom || a.editor.serialPort != "" {
		t.Fatalf("not typed: custom %v, port %q", a.editor.serialPortCustom, a.editor.serialPort)
	}
	typeInto(t, tt, locales.FieldSerialPort(), "COM9")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	if got := a.hosts.Hosts[0].SerialPort().Port; got != "COM9" {
		t.Errorf("the saved port %q", got)
	}
}

// A serial host without a port is not saved: the page says what is
// missing, and the editor stays open.
func TestEditorSerialNeedsPort(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.ProtocolSerial()); err != nil {
		t.Fatal(err)
	}
	// Typed, and left empty, whatever ports the computer has.
	a.editor.serialPortCustom, a.editor.serialPort = true, ""
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.windowOpen(WindowEditor) {
		t.Fatal("saved without a port")
	}
	if !tt.HasText(locales.ErrSerialPortRequired()) {
		t.Errorf("no error; texts %q", tt.Texts())
	}
	if len(a.hosts.Hosts) != 0 {
		t.Errorf("a host was saved: %v", a.hosts.Hosts)
	}
}

// A Telnet host is saved with its address and port, 23 unless typed.
func TestEditorSavesTelnetHost(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("Telnet"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !tt.HasText(locales.EditorPageTelnet()) {
		t.Fatalf("no Telnet page; texts %q", tt.Texts())
	}
	if a.editor.port != "23" {
		t.Fatalf("the port %q", a.editor.port)
	}
	if tt.HasText(locales.FieldUser()) || tt.HasText(locales.FieldPassword()) {
		t.Errorf("a Telnet page asks for a login; texts %q", tt.Texts())
	}
	typeInto(t, tt, locales.FieldName(), "router")
	typeInto(t, tt, locales.FieldHost(), "10.0.0.9")
	typeInto(t, tt, locales.FieldPort(), "2323")
	saveShot(t, tt, "editor-telnet-en.png")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	h := a.hosts.Hosts[0]
	if h.ProtocolOrDefault() != config.ProtocolTelnet || !h.IsTelnet() {
		t.Fatalf("host %+v", h)
	}
	if h.Addr != "10.0.0.9" || h.Port != 2323 || h.Target() != "10.0.0.9:2323" {
		t.Errorf("host %+v", h)
	}
	if !h.NoMonitor || h.User != "" {
		t.Errorf("host %+v", h)
	}
	if h.Serial != nil {
		t.Errorf("a Telnet host kept serial settings: %+v", h.Serial)
	}
}
