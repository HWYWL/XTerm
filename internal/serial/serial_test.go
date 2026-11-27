package serial

import (
	"testing"

	"go.bug.st/serial"
)

// The line's settings are the library's names: the editor's words, in the
// enums the port takes.
func TestLineSettings(t *testing.T) {
	for _, c := range []struct {
		name string
		got  serial.Parity
	}{
		{"none", serial.NoParity},
		{"even", serial.EvenParity},
		{"ODD", serial.OddParity},
		{"mark", serial.MarkParity},
		{"space", serial.SpaceParity},
		{"", serial.NoParity},
		{"other", serial.NoParity},
	} {
		if got := parity(c.name); got != c.got {
			t.Errorf("parity %q: %v", c.name, got)
		}
	}
	for _, c := range []struct {
		name string
		got  serial.StopBits
	}{
		{"1", serial.OneStopBit},
		{"1.5", serial.OnePointFiveStopBits},
		{"2", serial.TwoStopBits},
		{"", serial.OneStopBit},
	} {
		if got := stopBits(c.name); got != c.got {
			t.Errorf("stop bits %q: %v", c.name, got)
		}
	}
}

// A port that is not there is an error, not a panic, and an empty port is
// said plainly.
func TestOpenMissingPort(t *testing.T) {
	if _, err := Open(Config{}); err == nil {
		t.Error("an empty port opened")
	}
	if _, err := Open(Config{Port: "COM256", Baud: 9600, DataBits: 8}); err == nil {
		t.Error("a port that is not there opened")
	}
}
