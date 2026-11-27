package config

import "testing"

// A serial host's fields left empty are the console defaults: 115200, 8
// data bits, no parity, one stop bit.
func TestSerialDefaults(t *testing.T) {
	s := (&Host{Protocol: ProtocolSerial, Serial: &SerialSettings{Port: "COM3"}}).SerialPort()
	if s.Port != "COM3" || s.Baud != 115200 || s.DataBits != 8 || s.Parity != "none" || s.StopBits != "1" {
		t.Errorf("defaults %+v", s)
	}
	// What the host names is kept.
	s = (&Host{Protocol: ProtocolSerial, Serial: &SerialSettings{
		Port: "/dev/ttyUSB0", Baud: 9600, DataBits: 7, Parity: "even", StopBits: "2",
	}}).SerialPort()
	if s.Baud != 9600 || s.DataBits != 7 || s.Parity != "even" || s.StopBits != "2" {
		t.Errorf("named settings %+v", s)
	}
	// A host without the struct at all is still a console of the defaults.
	if s := (&Host{Protocol: ProtocolSerial}).SerialPort(); s.Baud != 115200 || s.Port != "" {
		t.Errorf("no settings %+v", s)
	}
}

// A serial port is its own target; a Telnet server's port is 23 unless
// the host says otherwise, and its target is the address with the port.
func TestRawTargets(t *testing.T) {
	serial := &Host{Name: "switch", Protocol: ProtocolSerial, Serial: &SerialSettings{Port: "COM3"}}
	if got := serial.Target(); got != "COM3" {
		t.Errorf("serial target %q", got)
	}
	if !serial.IsSerial() || serial.IsTelnet() || !serial.IsRaw() {
		t.Errorf("a serial host is serial, not Telnet")
	}

	// Without a name a serial host is called by its port, a Telnet host by
	// its address.
	unnamed := &Host{Protocol: ProtocolSerial, Serial: &SerialSettings{Port: "COM7"}}
	if got := unnamed.Title(); got != "COM7" {
		t.Errorf("a serial host's title %q", got)
	}
	if got := (&Host{Protocol: ProtocolTelnet, Addr: "10.0.0.9"}).Title(); got != "10.0.0.9" {
		t.Errorf("a Telnet host's title %q", got)
	}

	telnet := &Host{Name: "router", Addr: "10.0.0.9", Protocol: ProtocolTelnet}
	if got := telnet.Target(); got != "10.0.0.9:23" {
		t.Errorf("Telnet target %q", got)
	}
	if telnet.PortOr23() != 23 {
		t.Errorf("Telnet's port %d", telnet.PortOr23())
	}
	telnet.Port = 2323
	if got := telnet.Target(); got != "10.0.0.9:2323" {
		t.Errorf("Telnet target %q", got)
	}
	if telnet.PortOr23() != 2323 {
		t.Errorf("Telnet's port %d", telnet.PortOr23())
	}
	if !telnet.IsTelnet() || telnet.IsSerial() || !telnet.IsRaw() {
		t.Errorf("a Telnet host is Telnet, not serial")
	}

	// An SSH host is neither, and does not take the raw pages.
	ssh := &Host{Protocol: ProtocolSSH, Addr: "10.0.0.1", Port: 22}
	if ssh.IsRaw() || ssh.IsSerial() || ssh.IsTelnet() {
		t.Errorf("an SSH host is raw")
	}
}

// Cloning a host copies its serial settings, rather than sharing the
// struct a saved host keeps.
func TestCloneCopiesSerial(t *testing.T) {
	h := &Host{ID: "a", Protocol: ProtocolSerial, Serial: &SerialSettings{Port: "COM3", Baud: 9600}}
	c := h.Clone()
	if c.Serial == h.Serial {
		t.Fatal("the same struct")
	}
	c.Serial.Port = "COM4"
	if h.Serial.Port != "COM3" {
		t.Errorf("the original's port %q", h.Serial.Port)
	}
}
