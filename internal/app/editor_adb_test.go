package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// An ADB device on the network is saved with its protocol, its address and
// the port it listens on, and its default port is ADB's.
func TestEditorSavesADBWireless(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("ADB"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if !tt.HasText(locales.EditorPageADB()) {
		t.Fatalf("no ADB page; texts %q", tt.Texts())
	}
	// A new host starts on a cable, and the network asks for its address.
	if a.editor.adbModeOrDefault() != config.ADBUS {
		t.Errorf("the mode starts as %q", a.editor.adbMode)
	}
	choose(t, tt, locales.FieldADBConnection(), locales.AdbWireless())
	tt.Frame()
	if a.editor.adbMode != config.ADBTCP {
		t.Fatalf("the mode %q", a.editor.adbMode)
	}
	if a.editor.port != "5555" {
		t.Errorf("the port starts as %q", a.editor.port)
	}
	typeInto(t, tt, locales.FieldName(), "phone")
	typeInto(t, tt, locales.FieldHost(), "192.168.1.20")
	saveShot(t, tt, "editor-adb-wireless-en.png")
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
	if h.ProtocolOrDefault() != config.ProtocolADB || !h.IsADB() {
		t.Fatalf("host %+v", h)
	}
	cfg := h.ADBConfig()
	if cfg.Mode != config.ADBTCP || !cfg.Wireless() {
		t.Errorf("the settings %+v", cfg)
	}
	if h.Addr != "192.168.1.20" || h.PortOr5555() != 5555 {
		t.Errorf("the address %q:%d", h.Addr, h.Port)
	}
	if h.Target() != "192.168.1.20:5555" || h.Title() != "phone" {
		t.Errorf("target %q, title %q", h.Target(), h.Title())
	}
	if h.User != "" || !h.NoMonitor || h.Redis != nil || h.Serial != nil {
		t.Errorf("host %+v", h)
	}
}

// A device on a cable keeps the serial `adb devices` shows, and needs no
// address.
func TestEditorSavesADBWired(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("ADB"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	typeInto(t, tt, locales.FieldName(), "box")
	typeInto(t, tt, locales.FieldADBDevice(), "R58M12345")
	saveShot(t, tt, "editor-adb-wired-en.png")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	h := a.hosts.Hosts[0]
	cfg := h.ADBConfig()
	if cfg.Mode != config.ADBUS || cfg.Wireless() || cfg.Device != "R58M12345" {
		t.Errorf("the settings %+v", cfg)
	}
	if h.Addr != "" || h.Port != 0 {
		t.Errorf("an address on a cable: %q:%d", h.Addr, h.Port)
	}
	if h.Target() != "R58M12345" {
		t.Errorf("target %q", h.Target())
	}
}

// A device on the network cannot be saved without an address, and the page
// it is on says so.
func TestEditorADBNeedsAddress(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("ADB"); err != nil {
		t.Fatal(err)
	}
	choose(t, tt, locales.FieldADBConnection(), locales.AdbWireless())
	tt.Frame()
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.windowOpen(WindowEditor) {
		t.Fatal("the editor closed with no address")
	}
	if a.editor.nav.Page != editorADB || !tt.HasText(locales.ErrHostRequired()) {
		t.Errorf("page %q; texts %q", a.editor.nav.Page, tt.Texts())
	}
	if len(a.hosts.Hosts) != 0 {
		t.Errorf("hosts %v", a.hosts.Hosts)
	}
}
