package config

import "strings"

// ProtocolADB is an Android device reached over ADB: its terminal is a
// shell of the device itself, as a serial port's is the line.
const ProtocolADB = "adb"

// The ways a device is reached: by a cable, through the ADB server of this
// computer, or by the device's own port on the network.
const (
	ADBUS  = "usb"
	ADBTCP = "tcp"
)

// ADBPort is the port a device on the network listens on, unless the
// editor says another.
const ADBPort = 5555

// ADBSettings are the fields of an Android host: how it is reached, and
// which device it is on a cable.
type ADBSettings struct {
	// Mode is ADBUS or ADBTCP; empty is ADBUS, as a host saved before the
	// field had one.
	Mode string `json:"mode,omitempty"`
	// Device is the serial of the device on a cable, as `adb devices`
	// shows it; empty takes the first one.
	Device string `json:"device,omitempty"`
}

// WithDefaults is the settings as the app uses them.
func (s ADBSettings) WithDefaults() ADBSettings {
	if s.Mode != ADBTCP {
		s.Mode = ADBUS
	}
	s.Device = strings.TrimSpace(s.Device)
	return s
}

// Wireless tells whether the device is reached over the network.
func (s ADBSettings) Wireless() bool { return s.WithDefaults().Mode == ADBTCP }

// IsADB tells whether the host is an Android device reached over ADB; false
// for nil.
func (h *Host) IsADB() bool { return h != nil && h.Protocol == ProtocolADB }

// ADBConfig is the settings of the host's device, with the defaults of the
// ones it does not name.
func (h *Host) ADBConfig() ADBSettings {
	if h == nil || h.ADB == nil {
		return ADBSettings{}.WithDefaults()
	}
	return h.ADB.WithDefaults()
}

// PortOr5555 is the host's port, 5555 when unset: ADB's on the network.
func (h *Host) PortOr5555() int {
	if h.Port == 0 {
		return ADBPort
	}
	return h.Port
}

// ADBTarget is the device as the app shows it: its address on the network,
// the serial of one on a cable, or "ADB" for whichever device is first.
func (h *Host) ADBTarget() string {
	if h == nil {
		return ""
	}
	c := h.ADBConfig()
	if c.Wireless() {
		return h.AddrPort(ADBPort)
	}
	if c.Device != "" {
		return c.Device
	}
	return "ADB"
}
