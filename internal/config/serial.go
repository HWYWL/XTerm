package config

// The protocols besides SSH and Redis (see redis.go and object.go).
const (
	// ProtocolSerial is a device on a serial port of this computer: a
	// switch's console, a router, a board.
	ProtocolSerial = "serial"
	// ProtocolTelnet is a server reached over Telnet, whose terminal is
	// the connection itself.
	ProtocolTelnet = "telnet"
)

// SerialSettings are the fields of a serial host: where the port is, and
// how the line is set. A field left empty is the default below, so that a
// host saved by an older build, or one whose port only was chosen, still
// opens.
type SerialSettings struct {
	// Port is the device: COM3 on Windows, /dev/ttyUSB0 or /dev/ttyS0
	// elsewhere.
	Port string `json:"port,omitempty"`
	// Baud is the speed of the line, 115200 when unset.
	Baud int `json:"baud,omitempty"`
	// DataBits is the bits of a character, 8 when unset.
	DataBits int `json:"dataBits,omitempty"`
	// Parity is "none", "even", "odd", "mark" or "space", none when
	// unset.
	Parity string `json:"parity,omitempty"`
	// StopBits is "1", "1.5" or "2", one when unset.
	StopBits string `json:"stopBits,omitempty"`
}

// The defaults of a serial host: what a console port usually is.
const (
	SerialBaud     = 115200
	SerialDataBits = 8
	SerialParity   = "none"
	SerialStopBits = "1"
)

// WithDefaults is the settings as the app uses them: a field left empty is
// the default.
func (s SerialSettings) WithDefaults() SerialSettings {
	if s.Baud <= 0 {
		s.Baud = SerialBaud
	}
	if s.DataBits <= 0 {
		s.DataBits = SerialDataBits
	}
	if s.Parity == "" {
		s.Parity = SerialParity
	}
	if s.StopBits == "" {
		s.StopBits = SerialStopBits
	}
	return s
}

// IsSerial tells whether the host is a device on a serial port; false for
// nil.
func (h *Host) IsSerial() bool { return h != nil && h.Protocol == ProtocolSerial }

// IsTelnet tells whether the host is reached over Telnet; false for nil.
func (h *Host) IsTelnet() bool { return h != nil && h.Protocol == ProtocolTelnet }

// IsRaw tells whether the host's terminal is the connection itself — a
// serial port, or a Telnet server — rather than a shell the app asked an
// SSH server for. Its terminal has no shell to follow, no files and no
// server to monitor.
func (h *Host) IsRaw() bool { return h.IsSerial() || h.IsTelnet() }

// PortOr23 is the host's port, 23 when unset: Telnet's.
func (h *Host) PortOr23() int {
	if h.Port == 0 {
		return 23
	}
	return h.Port
}

// SerialPort is the settings of the host's serial port, with the defaults
// of the ones it does not name.
func (h *Host) SerialPort() SerialSettings {
	if h == nil || h.Serial == nil {
		return SerialSettings{}.WithDefaults()
	}
	return h.Serial.WithDefaults()
}
