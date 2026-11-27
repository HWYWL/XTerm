package app

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"finallyshell/internal/adb"
	"finallyshell/internal/config"
	"finallyshell/locales"
)

// The download is offered where a cable's device fails, and the bar says
// how far it has come.
func TestADBToolsDownload(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("ADB"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	old := installTools
	t.Cleanup(func() { installTools = old })
	dirs := make(chan string, 1)
	installTools = func(ctx context.Context, dir string, progress func(adb.Progress)) (string, error) {
		dirs <- dir
		progress(adb.Progress{Stage: adb.StageDownload, Done: 4 << 20, Total: 10 << 20})
		time.Sleep(20 * time.Millisecond)
		progress(adb.Progress{Stage: adb.StageUnpack})
		return filepath.Join(dir, "platform-tools", "adb"), nil
	}
	tt.Frame()
	if err := tt.Click(locales.AdbToolsDownload()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	p.until(t, "the download", func() bool { return !a.adbtools.busy && a.adbtools.path != "" })
	if a.adbtools.err != "" {
		t.Fatalf("the download failed: %s", a.adbtools.err)
	}
	if dir := <-dirs; dir != config.Dir() {
		t.Errorf("unpacked into %q, want %q", dir, config.Dir())
	}
	tt.Frame()
	if !tt.HasText(locales.AdbToolsReady()) {
		t.Errorf("the page says nothing of the tools; texts %q", tt.Texts())
	}
}

// A download that fails says why, and offers another try.
func TestADBToolsFailed(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("ADB"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	old := installTools
	t.Cleanup(func() { installTools = old })
	installTools = func(ctx context.Context, dir string, progress func(adb.Progress)) (string, error) {
		return "", io.ErrUnexpectedEOF
	}
	tt.Frame()
	if err := tt.Click(locales.AdbToolsDownload()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	p.until(t, "the download", func() bool { return !a.adbtools.busy && a.adbtools.err != "" })
	tt.Frame()
	if !tt.HasText(locales.AdbToolsRetry()) {
		t.Errorf("no retry; texts %q", tt.Texts())
	}
}

// A device on a cable that has no ADB server says so over the terminal, and
// offers the download there.
func TestADBToolsInBanner(t *testing.T) {
	a, p, tt := rawApp(t)
	oldOpen, oldBin, oldInstall := openADB, adbBinary, installTools
	t.Cleanup(func() { openADB, adbBinary, installTools = oldOpen, oldBin, oldInstall })
	openADB = func(ctx context.Context, o adb.Options) (io.ReadWriteCloser, error) {
		return nil, adb.ErrNoServer
	}
	adbBinary = func() (string, bool) { return "", false }
	started := make(chan struct{}, 1)
	installTools = func(ctx context.Context, dir string, progress func(adb.Progress)) (string, error) {
		started <- struct{}{}
		return "", errors.New("stopped by the test")
	}
	h := &config.Host{Name: "phone", Protocol: config.ProtocolADB,
		ADB: &config.ADBSettings{Mode: config.ADBUS}}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	s := a.current()
	p.until(t, "the failure", func() bool { return s.status == statusFailed })
	tt.Frame()
	if !tt.HasText(locales.AdbToolsDownload()) {
		t.Fatalf("no offer of the tools; texts %q", tt.Texts())
	}
	if err := tt.Click(locales.AdbToolsDownload()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Error("the download was not started")
	}
}

// The editor offers the tools on the page of a device on a cable.
func TestADBToolsInEditor(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click("ADB"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !tt.HasText(locales.AdbToolsDownload()) {
		t.Errorf("no offer of the tools; texts %q", tt.Texts())
	}
}
