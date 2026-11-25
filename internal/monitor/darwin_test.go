package monitor

import (
	"math"
	"testing"
	"time"
)

// TestParseDarwin: a Mac's script is read into the same Stats as a
// Linux server's: its boot time as the uptime, top's idle as the CPU, the
// active, wired and compressed pages as the memory in use, the swapusage
// sizes, df's disks, netstat's counters as /proc/net/dev's, and the
// counts.
func TestParseDarwin(t *testing.T) {
	const (
		gib = 1 << 30
		mib = 1 << 20
	)
	parts := []string{
		"{ sec = 1690000000, usec = 0 } Mon Nov 20 10:00:00 2023",
		"{ 1.20 1.30 1.10 }",
		"CPU usage: 3.12% user, 5.20% sys, 91.67% idle ",
		`Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               1000.
Pages active:                             2000.
Pages inactive:                            500.
Pages speculative:                         100.
Pages wired down:                          300.
Pages occupied by compressor:               50.`,
		"17179869184",
		"total = 2048.00M  used = 100.00M  free = 1948.00M",
		"Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/disk1s1 1000 400 600 40% /",
		`Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll
en0       1500  <Link#4>    a4:83:e7:00:00:00     1000     0       5000      800     0       3000     0
lo0      16384  <Link#1>                          100     0       1000      100     0       1000     0
`,
		"420",
		"2",
	}
	var st Stats
	var net netState
	parseDarwin(&st, parts, &net, time.Unix(1690003600, 0))

	if st.Uptime != time.Hour {
		t.Errorf("uptime %v", st.Uptime)
	}
	if st.Load != [3]float64{1.2, 1.3, 1.1} {
		t.Errorf("load %v", st.Load)
	}
	if math.Abs(st.CPU-(100-91.67)) > 0.01 {
		t.Errorf("cpu %v", st.CPU)
	}
	if st.MemTotal != 16*gib || st.MemUsed != (2000+300+50)*16384 {
		t.Errorf("memory %d of %d", st.MemUsed, st.MemTotal)
	}
	if st.SwapTotal != 2048*mib || st.SwapUsed != 100*mib {
		t.Errorf("swap %d of %d", st.SwapUsed, st.SwapTotal)
	}
	if len(st.Disks) != 1 || st.Disks[0].Mount != "/" || st.Disks[0].Avail != 600*1024 {
		t.Errorf("disks %+v", st.Disks)
	}
	if st.Iface != "en0" || len(st.Ifaces) != 1 || st.Ifaces[0].Name != "en0" {
		t.Errorf("iface %q %+v", st.Iface, st.Ifaces)
	}
	if st.ProcTotal != 420 || st.Users != 2 {
		t.Errorf("counts %d %d", st.ProcTotal, st.Users)
	}
}
