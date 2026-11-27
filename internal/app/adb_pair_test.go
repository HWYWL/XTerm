package app

import (
	"context"
	"crypto"
	"errors"
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"
)

// adbEditor is an editor on a device on the network, with the pairing
// details of its first connection.
func adbEditor() *connEditor {
	return &connEditor{protocol: editorADB, addr: "172.16.64.41", adbMode: config.ADBTCP,
		adbPairPort: "41234", adbPairCode: "385629"}
}

// adbPairingTest replaces the pairing and the adb of the computer for a
// test, and puts them back after it.
func adbPairingTest(t *testing.T, native func(context.Context, string, string, crypto.Signer) error) (runs *[]string, bin *bool) {
	t.Helper()
	oldPair, oldBin, oldRun := pairADB, adbBinary, runADB
	t.Cleanup(func() { pairADB, adbBinary, runADB = oldPair, oldBin, oldRun })
	list := &[]string{}
	have := new(bool)
	pairADB = native
	adbBinary = func() (string, bool) { return "/usr/bin/adb", *have }
	runADB = func(ctx context.Context, path string, args []string, stdin string) ([]byte, error) {
		*list = append(*list, strings.Join(args, " ")+" | "+stdin)
		return []byte("Successfully paired to 172.16.64.41:41234\n"), nil
	}
	return list, have
}

// The app pairs the device itself, with the code and the key it connects
// with; no adb is needed.
func TestADBPairNative(t *testing.T) {
	a, p := testApp(t)
	ed := adbEditor()
	var gotAddr, gotCode string
	list, have := adbPairingTest(t, func(ctx context.Context, addr, code string, key crypto.Signer) error {
		gotAddr, gotCode = addr, code
		if key == nil {
			t.Error("the pairing offered no key")
		}
		return nil
	})
	*have = false // and adb is not even installed
	a.adbPair(ed)
	p.until(t, "the pairing", func() bool { return !ed.adbPairing })
	if gotAddr != "172.16.64.41:41234" || gotCode != "385629" {
		t.Errorf("the device %q, the code %q", gotAddr, gotCode)
	}
	if len(*list) != 0 {
		t.Errorf("adb was run as well: %q", *list)
	}
	if ed.adbPairErr || ed.adbPairMsg != locales.AdbPairOK() {
		t.Errorf("the page says %q, an error %v", ed.adbPairMsg, ed.adbPairErr)
	}
}

// A device the app's own exchange does not get through with is paired with
// the adb of the computer, if there is one.
func TestADBPairFallsBackToAdb(t *testing.T) {
	a, p := testApp(t)
	ed := adbEditor()
	list, have := adbPairingTest(t, func(context.Context, string, string, crypto.Signer) error {
		return errors.New("the device refused the pairing")
	})
	*have = true
	a.adbPair(ed)
	p.until(t, "the pairing", func() bool { return !ed.adbPairing })
	if len(*list) != 1 || !strings.HasPrefix((*list)[0], "pair 172.16.64.41:41234 385629 | 385629") {
		t.Errorf("adb ran as %q", *list)
	}
	if ed.adbPairErr || ed.adbPairMsg != locales.AdbPairOK() {
		t.Errorf("the page says %q, an error %v", ed.adbPairMsg, ed.adbPairErr)
	}
}

// A code the device does not take is said in the app's words, with adb's
// own when it was tried too, and the editor stays as it was.
func TestADBPairFailed(t *testing.T) {
	a, p := testApp(t)
	ed := adbEditor()
	_, have := adbPairingTest(t, func(context.Context, string, string, crypto.Signer) error {
		return errors.New("the code was not the device's")
	})
	*have = true
	oldRun := runADB
	t.Cleanup(func() { runADB = oldRun })
	runADB = func(ctx context.Context, path string, args []string, stdin string) ([]byte, error) {
		return []byte("Enter pairing code: \nFailed: Wrong password\n"), errors.New("exit status 1")
	}
	a.adbPair(ed)
	p.until(t, "the pairing", func() bool { return !ed.adbPairing })
	want := locales.AdbPairFailed("the code was not the device's · adb: Failed: Wrong password")
	if !ed.adbPairErr || ed.adbPairMsg != want {
		t.Errorf("the page says %q, an error %v", ed.adbPairMsg, ed.adbPairErr)
	}
	if ed.adbPairPort != "41234" || ed.adbPairCode != "385629" {
		t.Errorf("the pairing fields were cleared: %+v", ed)
	}
}

// Without adb, a pairing the app could not make is reported as it came.
func TestADBPairNoAdb(t *testing.T) {
	a, p := testApp(t)
	ed := adbEditor()
	list, have := adbPairingTest(t, func(context.Context, string, string, crypto.Signer) error {
		return errors.New("no route to host")
	})
	*have = false
	a.adbPair(ed)
	p.until(t, "the pairing", func() bool { return !ed.adbPairing })
	if len(*list) != 0 {
		t.Errorf("adb was run: %q", *list)
	}
	if !ed.adbPairErr || ed.adbPairMsg != locales.AdbPairFailed("no route to host") {
		t.Errorf("the page says %q, an error %v", ed.adbPairMsg, ed.adbPairErr)
	}
}

// The pairing fields are the page's own: nothing is saved of them into the
// host, and a device on a cable has no pairing to do.
func TestADBPairNotSaved(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("ADB"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	// A device on a cable has no pairing section.
	if tt.HasText(locales.AdbPair()) {
		t.Errorf("a pairing section on a cable; texts %q", tt.Texts())
	}
	choose(t, tt, locales.FieldADBConnection(), locales.AdbWireless())
	tt.Frame()
	if !tt.HasText(locales.AdbPair()) || !tt.HasText(locales.AdbPairPort()) ||
		!tt.HasText(locales.AdbPairCode()) || !tt.HasText(locales.AdbPairButton()) {
		t.Errorf("no pairing section; texts %q", tt.Texts())
	}
	typeInto(t, tt, locales.FieldName(), "phone")
	typeInto(t, tt, locales.FieldHost(), "172.16.64.41")
	typeInto(t, tt, locales.AdbPairPort(), "41234")
	typeInto(t, tt, locales.AdbPairCode(), "385629")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	h := a.hosts.Hosts[0]
	if h.Addr != "172.16.64.41" || h.PortOr5555() != 5555 {
		t.Errorf("the host %+v", h)
	}
	if h.ADB == nil || h.ADB.Mode != config.ADBTCP || h.ADB.Device != "" {
		t.Errorf("the ADB settings %+v", h.ADB)
	}
}
