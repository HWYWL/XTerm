package config

import (
	"encoding/json"
	"testing"
)

// An Android host reaches its device on a cable unless it says otherwise,
// and shows that device as its target.
func TestADBSettings(t *testing.T) {
	var s ADBSettings
	if got := s.WithDefaults(); got.Mode != ADBUS || got.Wireless() {
		t.Errorf("the defaults %+v", got)
	}
	if !(ADBSettings{Mode: ADBTCP}).Wireless() {
		t.Error("a device on the network is not wireless")
	}
	if got := (ADBSettings{Device: " R58M "}).WithDefaults().Device; got != "R58M" {
		t.Errorf("the serial %q", got)
	}

	wireless := &Host{Protocol: ProtocolADB, Addr: "192.168.1.20",
		ADB: &ADBSettings{Mode: ADBTCP}}
	if !wireless.IsADB() {
		t.Error("the host is not an ADB one")
	}
	if wireless.PortOr5555() != ADBPort || wireless.ADBTarget() != "192.168.1.20:5555" {
		t.Errorf("target %q", wireless.ADBTarget())
	}
	if wireless.Title() != "192.168.1.20:5555" {
		t.Errorf("title %q", wireless.Title())
	}
	cable := &Host{Protocol: ProtocolADB, ADB: &ADBSettings{Device: "R58M"}}
	if cable.ADBTarget() != "R58M" {
		t.Errorf("target %q", cable.ADBTarget())
	}
	if first := (&Host{Protocol: ProtocolADB}); first.ADBTarget() != "ADB" {
		t.Errorf("target %q", first.ADBTarget())
	}
	if named := (&Host{Name: "phone", Protocol: ProtocolADB, Addr: "10.0.0.2"}); named.Title() != "phone" {
		t.Errorf("title %q", named.Title())
	}
	var nilHost *Host
	if nilHost.IsADB() || nilHost.ADBTarget() != "" {
		t.Error("a nil host is an ADB one")
	}
}

// The settings of the device are saved with the host, and a file without
// them reads a device on a cable.
func TestADBSettingsRoundTrip(t *testing.T) {
	h := &Host{ID: "p", Protocol: ProtocolADB, Addr: "10.0.0.7", Port: 5556,
		ADB: &ADBSettings{Mode: ADBTCP, Device: "R58M"}}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var back Host
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if got := back.ADBConfig(); got.Mode != ADBTCP || got.Device != "R58M" {
		t.Errorf("the settings read back as %+v", got)
	}
	if back.PortOr5555() != 5556 {
		t.Errorf("the port %d", back.PortOr5555())
	}
	old := &Host{Protocol: ProtocolADB}
	if got := old.ADBConfig(); got.Mode != ADBUS || got.Wireless() {
		t.Errorf("a file without the settings reads %+v", got)
	}
}
