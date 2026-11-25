package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
)

// fakeSysInfo answers the app's reads when the test says, one at a time.
type fakeSysInfo struct{ reads chan sysInfoRead }

type sysInfoRead struct {
	ctx   context.Context
	reply chan sysInfoReply
}

type sysInfoReply struct {
	info monitor.SysInfo
	err  error
}

func newFakeSysInfo(a *App) *fakeSysInfo {
	f := &fakeSysInfo{reads: make(chan sysInfoRead, 8)}
	a.readSysInfo = func(ctx context.Context, _ *ssh.Client) (monitor.SysInfo, error) {
		r := sysInfoRead{ctx, make(chan sysInfoReply, 1)}
		f.reads <- r
		select {
		case rep := <-r.reply:
			return rep.info, rep.err
		case <-ctx.Done():
			return monitor.SysInfo{}, ctx.Err()
		}
	}
	return f
}

// next is the read the app started last.
func (f *fakeSysInfo) next(t *testing.T) sysInfoRead {
	t.Helper()
	select {
	case r := <-f.reads:
		return r
	default:
		t.Fatal("no read started")
	}
	return sysInfoRead{}
}

func debianSample(t *testing.T) monitor.SysInfo {
	data, err := os.ReadFile("../monitor/testdata/sysinfo-debian.txt")
	if err != nil {
		t.Fatal(err)
	}
	return monitor.ParseSysInfo(string(data))
}

// clickButton clicks the button named name, where text of the same name
// shows elsewhere too.
func clickButton(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	b, ok := tt.AccessByRole(ui.RoleButton, name)
	if !ok {
		t.Fatalf("no %q button; texts %q", name, tt.Texts())
	}
	tt.ClickAt(b.Bounds.X+b.Bounds.W/2, b.Bounds.Y+b.Bounds.H/2)
}

func TestSidePanelSkeleton(t *testing.T) {
	a, p := testApp(t)
	a.setLeftPage(leftSystem) // the monitor's page
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	for _, want := range []string{"IP -", "Up -", "Load -", "0%", locales.SystemInfo(), locales.DiskPath()} {
		if !tt.HasText(want) {
			t.Errorf("English skeleton: missing %q; texts %q", want, tt.Texts())
		}
	}
	if b, _ := tt.AccessByRole(ui.RoleButton, locales.SystemInfo()); !b.Disabled {
		t.Error("System info enabled without a session")
	}
	saveShot(t, tt, "skeleton-en.png")
	a.setLanguage("zh-Hans")
	tt.Frame()
	for _, want := range []string{"运行 -", "负载 -", "系统信息", "可用/大小"} {
		if !tt.HasText(want) {
			t.Errorf("Chinese skeleton: missing %q", want)
		}
	}
	saveShot(t, tt, "skeleton-zh.png")

	// A session still connecting shows its state over the skeleton.
	demoSession(a, "db-1", "10.0.0.12").status = statusConnecting
	a.active = 0
	tt.Frame()
	if !tt.HasText("运行 -") || !tt.HasText(locales.StatusConnecting()) {
		t.Errorf("connecting: texts %q", tt.Texts())
	}
}

func TestSysInfoDialog(t *testing.T) {
	a, p := testApp(t)
	demoSession(a, "web-1", "45.56.78.90")
	a.active = 0
	a.setLeftPage(leftSystem) // the monitor's page
	f := newFakeSysInfo(a)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()

	clickButton(t, tt, locales.SystemInfo())
	if !a.sysInfo.open || !a.sysInfo.loading || !tt.HasText(locales.ReadingServerInfo()) {
		t.Fatalf("not loading: %+v; texts %q", a.sysInfo, tt.Texts())
	}
	saveShot(t, tt, "sysinfo-loading-en.png")
	f.next(t).reply <- sysInfoReply{info: debianSample(t)}
	p.until(t, "the system info", func() bool { return !a.sysInfo.loading })
	for _, want := range []string{locales.SysOverview(), locales.SysCpuUsage(), locales.SysNetwork(), locales.SysFilesystems(),
		"Debian GNU/Linux 12 (bookworm)", "6.1.0-18-amd64", "web-1", "x86_64",
		"0.08 0.12 0.09", "up 410 days, 0 hours",
		"Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz", "4", "2397.222", "35840 KB", "4794.44",
		"NVIDIA Corporation GA104 [GeForce RTX 3070]",
		"1.2%", "98.4%", "3.8 GB", "1.9 GB", "2.2 GB",
		"ens3", "117.7 MB", "941.9 MB",
		"/dev/vda1", "25.0 GB", "/run"} {
		if !tt.HasText(want) {
			t.Errorf("English: missing %q; texts %q", want, tt.Texts())
		}
	}
	saveShot(t, tt, "sysinfo-en.png")

	clickButton(t, tt, locales.Copy())
	got := tt.Clipboard()
	for _, want := range []string{"Hostname: web-1\n", "OS: Debian GNU/Linux 12 (bookworm)\n", "CPU:\n  Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz\n", "CPU usage:\n", "Network interfaces:\n", "ens3  Sent 117.7 MB  Received 941.9 MB\n", "Filesystems:\n", "/dev/vda1  11.8 GB  /\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
	// The copy says so in a toast, as the IP's does.
	tt.Advance(300 * time.Millisecond)
	if !tt.HasText(locales.Copied()) {
		t.Errorf("no toast; texts %q", tt.Texts())
	}

	// Closed, then opened in Chinese: it reads again.
	tt.Key(0, ui.KeyEscape)
	if a.sysInfo.open {
		t.Fatal("Escape left the dialog open")
	}
	a.setLanguage("zh-Hans")
	tt.Frame()
	clickButton(t, tt, "系统信息")
	f.next(t).reply <- sysInfoReply{info: debianSample(t)}
	p.until(t, "the system info", func() bool { return !a.sysInfo.loading })
	for _, want := range []string{"主机名", "内核", "概览", "处理器", "CPU 占用", "内存", "交换", "网络接口", "文件系统",
		"web-1", "up 410 days, 0 hours", "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz"} {
		if !tt.HasText(want) {
			t.Errorf("Chinese: missing %q; texts %q", want, tt.Texts())
		}
	}
	saveShot(t, tt, "sysinfo-zh.png")
	tt.Key(0, ui.KeyEscape)

	// An error, then a retry that works.
	a.setLanguage("en")
	tt.Frame()
	clickButton(t, tt, locales.SystemInfo())
	f.next(t).reply <- sysInfoReply{err: monitor.ErrSysInfoOutput}
	p.until(t, "the error", func() bool { return !a.sysInfo.loading })
	if want := locales.SysInfoFailed(locales.SysInfoNoShell()); !tt.HasText(want) {
		t.Errorf("no error shown; texts %q", tt.Texts())
	}
	saveShot(t, tt, "sysinfo-error-en.png")
	clickButton(t, tt, locales.Retry())
	f.next(t).reply <- sysInfoReply{info: debianSample(t)}
	p.until(t, "the retry", func() bool { return !a.sysInfo.loading })
	if !tt.HasText("x86_64") {
		t.Errorf("retry not shown; texts %q", tt.Texts())
	}
}

// Results of a read that was closed, reopened, or whose session went
// away are dropped.
func TestSysInfoStaleReads(t *testing.T) {
	a, p := testApp(t)
	web := demoSession(a, "web-1", "45.56.78.90")
	demoSession(a, "db-1", "10.0.0.12")
	a.active = 0
	a.setLeftPage(leftSystem) // the monitor's page
	f := newFakeSysInfo(a)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()

	// Opened twice quickly: the first read is canceled, its answer dropped.
	clickButton(t, tt, locales.SystemInfo())
	first := f.next(t)
	// The dialog covers the button: Escape closes it, as the Close button.
	tt.Key(0, ui.KeyEscape)
	if a.sysInfo.open {
		t.Fatal("Escape left it open")
	}
	clickButton(t, tt, locales.SystemInfo())
	second := f.next(t)
	if first.ctx.Err() == nil {
		t.Error("the first read was not canceled")
	}
	first.reply <- sysInfoReply{info: monitor.SysInfo{Hostname: "stale"}}
	second.reply <- sysInfoReply{info: monitor.SysInfo{Hostname: "fresh"}}
	p.until(t, "the second read", func() bool { return !a.sysInfo.loading })
	p.until(t, "the posts", func() bool { return len(p.posts) == 0 })
	if a.sysInfo.info.Hostname != "fresh" || tt.HasText("stale") {
		t.Errorf("shown %q", a.sysInfo.info.Hostname)
	}

	// The session closes mid-read: the dialog goes, and the other
	// session gets nothing of it.
	tt.Key(0, ui.KeyEscape)
	clickButton(t, tt, locales.SystemInfo())
	pending := f.next(t)
	a.closeSession(web)
	tt.Frame()
	if a.sysInfo.open {
		t.Fatal("open after its session closed")
	}
	pending.reply <- sysInfoReply{err: errors.New("connection closed")}
	p.until(t, "the posts", func() bool { return len(p.posts) == 0 })
	tt.Frame()
	if a.sysInfo.err != nil || a.sysInfo.info.Hostname != "" || a.sysInfo.open {
		t.Errorf("stale state %+v", a.sysInfo)
	}

	// Switching sessions closes it too.
	clickButton(t, tt, locales.SystemInfo())
	f.next(t).reply <- sysInfoReply{info: monitor.SysInfo{Hostname: "db"}}
	demoSession(a, "web-2", "45.56.78.91")
	a.cycle(1)
	tt.Frame()
	if a.sysInfo.open {
		t.Error("open for another session")
	}
	p.until(t, "the posts", func() bool { return len(p.posts) == 0 })
	if a.sysInfo.info.Hostname == "db" {
		t.Error("the answer came after the switch and was kept")
	}
}

// A host with the monitor off opens no exec channel: the side panel says
// so over the skeleton, and the dialog reads nothing.
// The copy after the IP puts the address in the clipboard, and says so in
// a toast that fades out two seconds later; the System info button ends
// the line.
func TestCopyServerAddress(t *testing.T) {
	a, p := testApp(t)
	demoSession(a, "web-1", "45.56.78.90")
	a.active = 0
	a.setLeftPage(leftSystem) // the monitor's page
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()

	// The copy beside the System info button at the end of the line, the IP
	// left the width before them.
	ip, cp, info := find(t, tt, "IP 45.56.78.90"), find(t, tt, locales.Copy()), find(t, tt, locales.SystemInfo())
	if cp.X < ip.X+ip.W || info.X < cp.X+cp.W {
		t.Errorf("IP at %+v, copy at %+v, System info at %+v", ip, cp, info)
	}
	if gap := info.X - (cp.X + cp.W); gap > 16 {
		t.Errorf("copy at %+v and System info at %+v, %v DIPs apart", cp, info, gap)
	}

	if err := tt.Click(locales.Copy()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if got := tt.Clipboard(); got != "45.56.78.90" {
		t.Errorf("clipboard %q", got)
	}
	if !tt.HasText(locales.Copied()) {
		t.Errorf("no toast; texts %q", tt.Texts())
	}
	tt.Advance(2 * time.Second)
	tt.Frame()
	if tt.HasText(locales.Copied()) {
		t.Error("the toast outlives its 2 seconds")
	}
}

func TestSysInfoMonitorOff(t *testing.T) {
	a, p := testApp(t)
	s := demoSession(a, "web-1", "45.56.78.90")
	s.Host.NoMonitor = true
	s.stats = monitor.Stats{}
	a.active = 0
	a.setLeftPage(leftSystem) // the monitor's page
	f := newFakeSysInfo(a)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	for _, want := range []string{"IP 45.56.78.90", locales.MonitorOff(), "Up -", "Load -"} {
		if !tt.HasText(want) {
			t.Errorf("side panel: missing %q; texts %q", want, tt.Texts())
		}
	}
	if tt.HasText(locales.ReadingServerInfo()) {
		t.Error("waiting for stats that never come")
	}
	saveShot(t, tt, "monitor-off-en.png")
	clickButton(t, tt, locales.SystemInfo())
	if !tt.HasText(locales.SysInfoOff()) {
		t.Errorf("no message; texts %q", tt.Texts())
	}
	if len(f.reads) != 0 {
		t.Error("read with the monitor off")
	}
	tt.Key(0, ui.KeyEscape)
	a.setLanguage("zh-Hans")
	tt.Frame()
	if !tt.HasText("此主机已关闭服务器监控") {
		t.Errorf("Chinese: texts %q", tt.Texts())
	}
	saveShot(t, tt, "monitor-off-zh.png")
}
