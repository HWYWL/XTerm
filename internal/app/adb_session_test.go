package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"finallyshell/internal/adb"
	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// An ADB host connects: its terminal is a shell of the device, what is
// typed goes to it, the tab has no bottom panel, its page names the device
// and how it is reached, and Disconnect closes the shell.
func TestADBSession(t *testing.T) {
	a, p, tt := rawApp(t)
	client, device := net.Pipe()
	t.Cleanup(func() { device.Close() })
	old := openADB
	t.Cleanup(func() { openADB = old })
	opened := make(chan adb.Options, 1)
	openADB = func(ctx context.Context, o adb.Options) (io.ReadWriteCloser, error) {
		opened <- o
		return client, nil
	}
	rx := make(chan string, 32)
	ended := make(chan struct{})
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := device.Read(buf)
			if n > 0 {
				rx <- string(buf[:n])
			}
			if err != nil {
				close(ended)
				return
			}
		}
	}()
	go device.Write([]byte("aosp:/ $ "))

	h := &config.Host{Name: "phone", Protocol: config.ProtocolADB, Addr: "192.168.1.20", Port: 5555,
		ADB: &config.ADBSettings{Mode: config.ADBTCP}}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	s := a.current()
	if s == nil || !s.isADB() {
		t.Fatalf("the session %v", s)
	}
	p.until(t, "the connection", func() bool { return s.status != statusConnecting })
	if s.status == statusFailed {
		t.Fatalf("the connection failed: %v", s.err)
	}
	untilScreen(t, p, s, "aosp:/ $")
	o := <-opened
	if !o.Wireless || o.Addr != "192.168.1.20:5555" || o.Term == "" || o.KeyPath == "" {
		t.Errorf("the device was reached as %+v", o)
	}
	// The shell is a terminal: typing goes to the device.
	tt.Frame()
	tt.Type("getprop ro.product.model")
	tt.Key(0, ui.KeyEnter)
	if got := readUntil(t, rx, "getprop"); !strings.Contains(got, "getprop ro.product.model") {
		t.Errorf("the device read %q", got)
	}
	// No bottom panel, and a page of the device's own.
	if why := a.noBottomPanel(); why != "" {
		t.Errorf("an ADB tab has no bottom panel: %s", why)
	}
	tt.Frame()
	if !tt.HasText(locales.TabFiles()) || !tt.HasText(locales.TabApps()) || !tt.HasText(locales.Ai()) {
		t.Errorf("no tabs; texts %q", tt.Texts())
	}
	if err := tt.Click(locales.TabApps()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if !tt.HasText(locales.AdbInstallDrop()) {
		t.Errorf("no drop zone; texts %q", tt.Texts())
	}
	a.setLeftPage(leftSystem)
	tt.Frame()
	if !tt.HasText(locales.AdbWireless()) || !tt.HasText("192.168.1.20") {
		t.Errorf("the side panel; texts %q", tt.Texts())
	}
	// The page has the link, the traffic, the packets and channels, the log
	// stream and the device metrics, whose switches turn collection on.
	for _, title := range []string{locales.TelnetLink(), locales.SerialTraffic(),
		locales.AdbPacketsTitle(), locales.AdbLogTitle(), locales.AdbMetricsTitle()} {
		if !tt.HasText(title) {
			t.Errorf("no %q in the ADB page; texts %q", title, tt.Texts())
		}
	}
	if err := tt.Click(locales.AdbLogOn()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if !a.adbMeter(s).view().Logcat {
		t.Error("the log stream was not turned on")
	}
	if !a.adbMeter(s).view().Metrics {
		t.Error("the device metrics are not on by default")
	}
	if err := tt.Click(locales.AdbMetricsOn()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if a.adbMeter(s).view().Metrics {
		t.Error("the device metrics did not turn off")
	}
	saveShot(t, tt, "adb-session-en.png")
	// Disconnect closes the shell: the device sees the end of it.
	a.disconnectSession(s)
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("the shell stayed open")
	}
	if s.status != statusClosed {
		t.Errorf("the session is %v", s.status)
	}
	for _, pane := range s.panes.panes() {
		if pane.wire != nil {
			t.Error("the pane still writes through the closed shell")
		}
	}
}

// A device that answers nothing is reported as a connect timeout, and the
// page says the key was not allowed rather than hiding it.
func TestADBUnauthorized(t *testing.T) {
	a, p, _ := rawApp(t)
	old := openADB
	t.Cleanup(func() { openADB = old })
	openADB = func(ctx context.Context, o adb.Options) (io.ReadWriteCloser, error) {
		<-ctx.Done() // as a device that asks, and the question is never answered
		return nil, adb.ErrUnauthorized
	}
	oldTimeout := adbDialTimeout
	adbDialTimeout = 20 * time.Millisecond
	t.Cleanup(func() { adbDialTimeout = oldTimeout })

	h := &config.Host{Name: "box", Protocol: config.ProtocolADB,
		ADB: &config.ADBSettings{Mode: config.ADBUS, Device: "R58M12345"}}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	s := a.current()
	p.until(t, "the failure", func() bool { return s.status == statusFailed })
	if msg := describeError(s.err); msg != locales.ErrADBUnauthorized() {
		t.Errorf("the failure reads %q", msg)
	}
}

// A device that drops the connection says what to do about it: wireless
// debugging needs pairing, and the raw kick is not shown.
func TestADBRejected(t *testing.T) {
	a, p, _ := rawApp(t)
	old := openADB
	t.Cleanup(func() { openADB = old })
	openADB = func(ctx context.Context, o adb.Options) (io.ReadWriteCloser, error) {
		return nil, fmt.Errorf("%w: kicked: kicked", adb.ErrRejected)
	}
	h := &config.Host{Name: "phone", Protocol: config.ProtocolADB, Addr: "172.16.64.41", Port: 42723,
		ADB: &config.ADBSettings{Mode: config.ADBTCP}}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	s := a.current()
	p.until(t, "the failure", func() bool { return s.status == statusFailed })
	if msg := describeError(s.err); msg != locales.ErrADBRejected() {
		t.Errorf("the failure reads %q", msg)
	}
}

// nopStream is a connection that says nothing, which a test uses where the
// shell itself is not what it tests.
type nopStream struct{}

func (nopStream) Read([]byte) (int, error)    { return 0, io.EOF }
func (nopStream) Write(p []byte) (int, error) { return len(p), nil }
func (nopStream) Close() error                { return nil }

// A device on a cable needs the ADB server of this computer: when it is not
// running and adb is there, the app starts it and tries once more.
func TestADBStartsServer(t *testing.T) {
	a, _ := testApp(t)
	oldOpen, oldBin, oldRun := openADB, adbBinary, runADB
	t.Cleanup(func() { openADB, adbBinary, runADB = oldOpen, oldBin, oldRun })
	tries := 0
	openADB = func(ctx context.Context, o adb.Options) (io.ReadWriteCloser, error) {
		tries++
		if tries == 1 {
			return nil, adb.ErrNoServer
		}
		return nopStream{}, nil
	}
	adbBinary = func() (string, bool) { return "/usr/bin/adb", true }
	var ran []string
	runADB = func(ctx context.Context, bin string, args []string, stdin string) ([]byte, error) {
		ran = append(ran, strings.Join(args, " "))
		return nil, nil
	}
	s := &Session{Host: &config.Host{Protocol: config.ProtocolADB,
		ADB: &config.ADBSettings{Mode: config.ADBUS}}}
	if _, err := a.dialADB(context.Background(), s); err != nil {
		t.Fatalf("the connection: %v", err)
	}
	if tries != 2 {
		t.Errorf("the device was dialled %d times", tries)
	}
	if len(ran) != 1 || ran[0] != "start-server" {
		t.Errorf("adb ran as %q", ran)
	}
}

// Without adb, a cable's device is reported as the missing server it is.
func TestADBNoServer(t *testing.T) {
	a, _ := testApp(t)
	oldOpen, oldBin := openADB, adbBinary
	t.Cleanup(func() { openADB, adbBinary = oldOpen, oldBin })
	openADB = func(ctx context.Context, o adb.Options) (io.ReadWriteCloser, error) {
		return nil, adb.ErrNoServer
	}
	adbBinary = func() (string, bool) { return "", false }
	s := &Session{Host: &config.Host{Protocol: config.ProtocolADB,
		ADB: &config.ADBSettings{Mode: config.ADBUS}}}
	_, err := a.dialADB(context.Background(), s)
	if !errors.Is(err, adb.ErrNoServer) {
		t.Fatalf("the failure: %v", err)
	}
	if msg := describeError(err); msg != locales.ErrADBNoServer() {
		t.Errorf("the failure reads %q", msg)
	}
}
