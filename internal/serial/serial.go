// Package serial opens a serial port of this computer — a console port, a
// board, a switch — as a connection the terminal reads and writes, with
// the line set as its host says.
package serial

import (
	"fmt"
	"io"
	"strings"

	"go.bug.st/serial"
)

// Port is the library's port: what Open returns as its connection. The
// status page asks it for the modem pins, and raises or lowers DTR and RTS
// (internal/serialstat).
type Port = serial.Port

// Config is a port and the line's settings, as a host saved them.
type Config struct {
	Port     string // COM3, /dev/ttyUSB0
	Baud     int
	DataBits int
	Parity   string // none, even, odd, mark, space
	StopBits string // 1, 1.5, 2
}

// Open opens the port, with the line set as c says, and returns it as the
// connection the terminal uses.
func Open(c Config) (io.ReadWriteCloser, error) {
	if strings.TrimSpace(c.Port) == "" {
		return nil, fmt.Errorf("no serial port")
	}
	mode := &serial.Mode{
		BaudRate: c.Baud,
		DataBits: c.DataBits,
		Parity:   parity(c.Parity),
		StopBits: stopBits(c.StopBits),
	}
	if mode.BaudRate <= 0 {
		mode.BaudRate = 115200
	}
	if mode.DataBits <= 0 {
		mode.DataBits = 8
	}
	port, err := serial.Open(c.Port, mode)
	if err != nil {
		return nil, err
	}
	return port, nil
}

// Ports lists the serial ports of this computer, which the editor offers
// as suggestions: COM3 on Windows, /dev/ttyUSB0 and such elsewhere.
func Ports() []string {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil
	}
	return ports
}

// parity is the port's parity as the library names it.
func parity(name string) serial.Parity {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "even":
		return serial.EvenParity
	case "odd":
		return serial.OddParity
	case "mark":
		return serial.MarkParity
	case "space":
		return serial.SpaceParity
	}
	return serial.NoParity
}

// stopBits is the port's stop bits as the library names them.
func stopBits(name string) serial.StopBits {
	switch strings.TrimSpace(name) {
	case "1.5":
		return serial.OnePointFiveStopBits
	case "2":
		return serial.TwoStopBits
	}
	return serial.OneStopBit
}
