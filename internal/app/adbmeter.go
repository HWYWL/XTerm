package app

// The System page of an ADB tab: the link and its versions, what the
// connection carried, the ADB packets and channels behind it, the log
// stream, and — when asked for — what the device itself reports.

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"finallyshell/internal/adb"
	"finallyshell/locales"
)

// adbMeterPoll is how often an ADB tab's page samples; tests shorten it.
var adbMeterPoll = time.Second

// adbDevicePoll is how often the device's own numbers are read, when the
// switch is on.
var adbDevicePoll = 5 * time.Second

// adbIdleWarn is how long the device may be quiet before the page says its
// shell has stopped.
var adbIdleWarn = 30 * time.Second

// adbMeter is what an ADB tab's System page shows (adbpanel.go).
type adbMeter struct {
	app  *App
	host string

	mu sync.Mutex
	// conn is the connection the page reports on: the pane's last opened.
	conn *adb.Conn
	addr string
	mode string // "USB" or the address of a device on the network
	// serial and state are what the ADB server says of the device, on a
	// cable.
	serial, state string
	attempts      int

	// The sampling: the bytes of the sample before, the curves, the peaks,
	// and the gap since the device last sent anything.
	sampledAt          time.Time
	lastRX, lastTX     uint64
	rxHist, txHist     []float64
	peakRx, peakTx     float64
	firstAt, lastFrame time.Time
	lastGap, maxGap    time.Duration
	st                 adb.Stats

	// The device's own numbers, read when the switch is on.
	metrics  bool
	dev      adbDevice
	devAt    time.Time
	devBusy  bool
	devErr   string
	lastStat [4]uint64 // the CPU's counters at the sample before

	// logcat is whether the log stream is read, for its counts.
	logcat bool
}

// adbDevice is what a poll of the device reported.
type adbDevice struct {
	Model, Android, SDK string
	Uptime              time.Duration
	CPU                 float64 // 0 to 1
	MemTotal, MemFree   uint64
	DataUsed, DataTotal uint64
	Procs               int
	Root                bool
	Battery             int
	Charging            bool
	BatteryTemp         float64 // °C
	SoCTemp             float64 // °C
}

// adbView is the meter as the page draws it: a copy, taken once a frame.
type adbView struct {
	Addr, Mode, Serial, State string
	Reconnects                int
	RX, TX                    uint64
	RXRate, TXRate            float64
	RXHist, TXHist            []float64
	PeakRX, PeakTX            float64
	LastFrame                 time.Time
	LastGap, MaxGap           time.Duration
	ST                        adb.Stats
	Device                    adbDevice
	DeviceOK                  bool
	DeviceErr                 string
	DeviceAt                  time.Time
	Metrics                   bool
	Logcat                    bool
}

// adbMeter is s's meter, made the first time it is asked for: a session
// keeps it across a reconnect.
func (a *App) adbMeter(s *Session) *adbMeter {
	if s.adb == nil {
		host := ""
		if s.Host != nil {
			host = s.Host.ID
		}
		s.adb = &adbMeter{app: a, host: host, metrics: true}
	}
	return s.adb
}

// noteAttempt counts a connection this session starts.
func (m *adbMeter) noteAttempt() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.attempts++
	m.mu.Unlock()
}

// attach is the connection the page reports on, with how it is reached.
func (m *adbMeter) attach(conn *adb.Conn, addr, mode string) {
	if m == nil || conn == nil {
		return
	}
	m.mu.Lock()
	m.conn, m.addr, m.mode = conn, addr, mode
	m.firstAt, m.lastFrame = time.Now(), time.Time{}
	m.mu.Unlock()
}

// detach forgets the connection: the socket is closed, and the page must
// not read it any more.
func (m *adbMeter) detach() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.conn != nil {
		m.st = m.conn.Stats()
		m.conn = nil
	}
	m.mu.Unlock()
}

// noteDevices keeps what the ADB server said of the device, on a cable.
func (m *adbMeter) noteDevices(serial, state string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.serial, m.state = serial, state
	m.mu.Unlock()
}

// sample takes a second's reading: the rates, the curves, and the gap
// since the device last sent anything.
func (m *adbMeter) sample(now time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	conn := m.conn
	var st adb.Stats
	if conn != nil {
		st = conn.Stats()
		m.st = st
	}
	if m.sampledAt.IsZero() {
		m.sampledAt, m.lastRX, m.lastTX = now, st.RX, st.TX
		m.mu.Unlock()
		return
	}
	seconds := now.Sub(m.sampledAt).Seconds()
	if seconds <= 0 {
		m.mu.Unlock()
		return
	}
	rxRate := float64(st.RX-m.lastRX) / seconds
	txRate := float64(st.TX-m.lastTX) / seconds
	m.rxHist = serialHist(m.rxHist, rxRate)
	m.txHist = serialHist(m.txHist, txRate)
	if rxRate > m.peakRx {
		m.peakRx = rxRate
	}
	if txRate > m.peakTx {
		m.peakTx = txRate
	}
	m.lastRX, m.lastTX, m.sampledAt = st.RX, st.TX, now
	if !st.LastRX.IsZero() && st.LastRX.After(m.lastFrame) {
		if !m.lastFrame.IsZero() {
			m.lastGap = st.LastRX.Sub(m.lastFrame)
			if m.lastGap > m.maxGap {
				m.maxGap = m.lastGap
			}
		}
		m.lastFrame = st.LastRX
	}
	m.mu.Unlock()
}

// view is the meter's copy for the page.
func (m *adbMeter) view() adbView {
	if m == nil {
		return adbView{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	v := adbView{
		Addr: m.addr, Mode: m.mode, Serial: m.serial, State: m.state,
		Reconnects: max(0, m.attempts-1),
		RX:         m.st.RX, TX: m.st.TX,
		RXHist: m.rxHist, TXHist: m.txHist,
		PeakRX: m.peakRx, PeakTX: m.peakTx,
		LastFrame: m.lastFrame, LastGap: m.lastGap, MaxGap: m.maxGap,
		ST:        m.st,
		Device:    m.dev,
		DeviceOK:  !m.devAt.IsZero(),
		DeviceErr: m.devErr,
		DeviceAt:  m.devAt,
		Metrics:   m.metrics,
		Logcat:    m.logcat,
	}
	if n := len(m.rxHist); n > 0 {
		v.RXRate = m.rxHist[n-1]
	}
	if n := len(m.txHist); n > 0 {
		v.TXRate = m.txHist[n-1]
	}
	return v
}

// startADBMeter samples s's meter once a second, reads the device's numbers
// when asked for, and redraws the page while the session is shown.
func (a *App) startADBMeter(s *Session, ctx context.Context) {
	m := a.adbMeter(s)
	m.sample(time.Now())
	poll := adbMeterPoll
	go func() {
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.post(func() {
					if ctx.Err() != nil {
						return
					}
					m.sample(time.Now())
					m.maybePollDevice(ctx)
					if a.connSession() == s {
						a.redraw()
					}
				})
			}
		}
	}()
}

// maybePollDevice reads the device's own numbers when the switch is on and
// the collection is due, on a goroutine of its own.
func (m *adbMeter) maybePollDevice(ctx context.Context) {
	m.mu.Lock()
	conn, due := m.conn, m.metrics && !m.devBusy && time.Since(m.devAt) >= adbDevicePoll
	if due {
		m.devBusy = true
	}
	m.mu.Unlock()
	if !due || conn == nil {
		return
	}
	go func() {
		dev, err := m.app.readADBDevice(ctx, conn, &m.lastStat)
		m.mu.Lock()
		m.devBusy = false
		if err != nil {
			m.devErr = err.Error()
		} else {
			m.dev, m.devAt, m.devErr = dev, time.Now(), ""
		}
		m.mu.Unlock()
	}()
}

// adbDeviceScript prints what the page reads of the device, one labelled
// section after another.
const adbDeviceScript = `echo M; getprop ro.product.model; getprop ro.build.version.release; getprop ro.build.version.sdk
echo T; cat /proc/uptime
echo C; head -1 /proc/stat
echo F; cat /proc/meminfo
echo D; df -k /data 2>/dev/null | tail -1
echo P; ls /proc | grep -c '^[0-9]'
echo U; id -u
echo B; dumpsys battery 2>/dev/null | grep -E 'level|status|temperature'
echo H; cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null
`

// readADBDevice runs the script over a channel of its own and reads it.
func (a *App) readADBDevice(ctx context.Context, conn *adb.Conn, last *[4]uint64) (adbDevice, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := conn.Exec(ctx, adbDeviceScript)
	if err != nil {
		return adbDevice{}, err
	}
	return parseADBDevice(out, last), nil
}

// parseADBDevice reads the script's sections into a device.
func parseADBDevice(out string, last *[4]uint64) adbDevice {
	var d adbDevice
	section := ""
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) == 1 && strings.Contains("MTCFDPUBH", line) {
			section = line
			continue
		}
		switch section {
		case "M":
			switch {
			case d.Model == "":
				d.Model = line
			case d.Android == "":
				d.Android = line
			case d.SDK == "":
				d.SDK = line
			}
		case "T":
			if f := strings.Fields(line); len(f) > 0 {
				if v, err := strconv.ParseFloat(f[0], 64); err == nil {
					d.Uptime = time.Duration(v * float64(time.Second))
				}
			}
		case "C":
			d.CPU, *last = adbCPU(line, *last)
		case "F":
			if f := strings.Fields(line); len(f) >= 2 {
				v, _ := strconv.ParseUint(f[1], 10, 64)
				switch f[0] {
				case "MemTotal:":
					d.MemTotal = v * 1024
				case "MemFree:":
					d.MemFree = v * 1024
				}
			}
		case "D":
			if f := strings.Fields(line); len(f) >= 3 {
				d.DataTotal, _ = strconv.ParseUint(f[1], 10, 64)
				avail, _ := strconv.ParseUint(f[3], 10, 64)
				if d.DataTotal >= avail {
					d.DataUsed = d.DataTotal - avail
				}
			}
		case "P":
			d.Procs, _ = strconv.Atoi(line)
		case "U":
			d.Root = line == "0"
		case "B":
			switch {
			case strings.HasPrefix(line, "level:"):
				d.Battery, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "level:")))
			case strings.HasPrefix(line, "status:"):
				d.Charging = strings.Contains(line, "charging") || strings.Contains(line, "full")
			case strings.HasPrefix(line, "temperature:"):
				if v, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "temperature:"))); err == nil {
					d.BatteryTemp = float64(v) / 10
				}
			}
		case "H":
			if v, err := strconv.Atoi(line); err == nil && v > 0 {
				d.SoCTemp = float64(v) / 1000
			}
		}
	}
	return d
}

// adbCPU is the busy share of the CPU between two reads of /proc/stat.
func adbCPU(line string, last [4]uint64) (float64, [4]uint64) {
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, last
	}
	var cur [4]uint64
	for i := 0; i < 4 && i+1 < len(f); i++ {
		cur[i], _ = strconv.ParseUint(f[i+1], 10, 64)
	}
	busy := cur[0] + cur[1] + cur[2]
	total := busy + cur[3]
	wasBusy := last[0] + last[1] + last[2]
	wasTotal := wasBusy + last[3]
	if wasTotal == 0 || total <= wasTotal {
		return 0, cur
	}
	return float64(busy-wasBusy) / float64(total-wasTotal), cur
}

// adbLinkState names the link as the page shows it, from the session's own
// state and the device's.
func adbLinkState(s *Session, v adbView) string {
	switch s.status {
	case statusConnecting:
		return locales.StatusConnecting()
	case statusConnected:
		if v.ST.Closed {
			return locales.TelnetDisconnected()
		}
		if v.ST.Timeouts > 0 && time.Since(v.LastFrame) > adbIdleWarn {
			return locales.TelnetReadTimeout()
		}
		return locales.TelnetConnected()
	case statusFailed:
		return locales.TelnetDisconnected()
	}
	return locales.TelnetDisconnected()
}

// adbSeconds is a duration as the page writes it.
func adbSeconds(d time.Duration) string { return serialGap(d) }

// adbPercent is a share, from 0 to 1, as the page writes it.
func adbPercent(f float64) string { return strconv.Itoa(int(f*100+0.5)) + "%" }
