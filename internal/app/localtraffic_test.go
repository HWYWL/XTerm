package app

import (
	"testing"

	"finallyshell/internal/monitor"
)

// TestParseNetDev sums the interfaces of /proc/net/dev, the loopback left
// out: its traffic is the app's own connections, counted twice.
func TestParseNetDev(t *testing.T) {
	const header = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
`
	tests := []struct {
		name string
		in   string
		rx   uint64
		tx   uint64
	}{
		{
			name: "loopback left out",
			in: header + `    lo: 1000 10 0 0 0 0 0 0 1000 10 0 0 0 0 0 0
`,
		},
		{
			name: "two interfaces summed",
			in: header + `  eth0: 4096 20 0 0 0 0 0 0 8192 30 0 0 0 0 0 0
  eth1: 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16
`,
			rx: 4097,
			tx: 8201,
		},
		{
			name: "a short line and a stray line are skipped",
			in: header + `  eth0: 4096 20 0 0 0 0 0 0 8192 30 0 0 0 0 0 0
  eth1: 1 2
not a line
`,
			rx: 4096,
			tx: 8192,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rx, tx := parseNetDev([]byte(tc.in))
			if rx != tc.rx || tx != tc.tx {
				t.Errorf("rx, tx = %d, %d, want %d, %d", rx, tx, tc.rx, tc.tx)
			}
		})
	}
}

// TestDiskUsedRatio: the ring shows the partitions as one disk, the used
// bytes over the total.
func TestDiskUsedRatio(t *testing.T) {
	tests := []struct {
		name  string
		disks []monitor.Disk
		want  float64
	}{
		{name: "no disks", want: 0},
		{name: "a disk half used", disks: []monitor.Disk{{Mount: "/", Avail: 50, Size: 100}}, want: 0.5},
		{
			name:  "two disks summed, not averaged",
			disks: []monitor.Disk{{Mount: "/", Avail: 25, Size: 100}, {Mount: "/data", Avail: 50, Size: 50}},
			want:  0.5,
		},
		{name: "avail above size counts as none used", disks: []monitor.Disk{{Mount: "/", Avail: 200, Size: 100}}, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := diskUsedRatio(tc.disks); got != tc.want {
				t.Errorf("ratio = %v, want %v", got, tc.want)
			}
		})
	}
}
