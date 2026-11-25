package monitor

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func readSample(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseSysInfoSamples(t *testing.T) {
	tests := []struct {
		file string
		want SysInfo
	}{
		{"sysinfo-debian.txt", SysInfo{
			Hostname: "web-1", OS: "Debian GNU/Linux 12 (bookworm)", KernelOS: "Linux", Kernel: "6.1.0-18-amd64", Arch: "x86_64",
			CPUModel: "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz", CPUs: 4, Cores: 2,
			MemTotal: 4009216 * 1024, SwapTotal: 2306044 * 1024, Uptime: 35425043 * time.Second,
			Addrs: []Addr{{"ens3", "45.56.78.90/24"}, {"ens3", "2600:3c01::f03c:93ff:fe4a:1b2c/64"}, {"docker0", "172.17.0.1/16"}},
			Disks: []BlockDevice{{Name: "vda", Size: 52428800 * 512}},

			Load: "0.08 0.12 0.09", UptimeUp: "up 410 days, 0 hours",
			IPs:     "45.56.78.90 2600:3c01::f03c:93ff:fe4a:1b2c",
			CPUFreq: "2397.222", CPUCache: "35840 KB", CPUBogo: "4794.44",
			GPU: "NVIDIA Corporation GA104 [GeForce RTX 3070]",
			Mem: Memory{Total: 4009216 * 1024, Available: 3200000 * 1024, Free: 1200000 * 1024, Buffers: 123456 * 1024, Cached: 1876543 * 1024},
			Swap: Memory{Total: 2306044 * 1024, Available: 2200000 * 1024, Free: 2200000 * 1024},
			Nets: []NetIface{{"lo", 1824, 1824}, {"ens3", 987654321, 123456789}},
			FS: []Filesystem{
				{"/dev/vda1", 26214400 * 1024, 12345678 * 1024, 12400000 * 1024, "/"},
				{"tmpfs", 2048000 * 1024, 0, 2048000 * 1024, "/run"},
			},
		}},
		// BusyBox: no lscpu, an arm64 cpuinfo without "model name".
		{"sysinfo-alpine.txt", SysInfo{
			Hostname: "pi-gate", OS: "Alpine Linux v3.20", KernelOS: "Linux", Kernel: "6.6.31-0-rpi", Arch: "aarch64",
			CPUModel: "Raspberry Pi 4 Model B Rev 1.4", CPUs: 4,
			MemTotal: 3884296 * 1024, Uptime: 93785 * time.Second,
			Addrs: []Addr{{"eth0", "192.168.1.40/24"}, {"wlan0", "192.168.1.41/24"}},
			Disks: []BlockDevice{{Name: "mmcblk0", Size: 62333952 * 512}, {Name: "sda", Size: 1953525168 * 512, Model: "Extreme SSD"}},

			Load: "0.15 0.10 0.05", UptimeUp: "up 1 day, 2 hours",
			IPs:      "192.168.1.40 192.168.1.41",
			CPUCache: "512 KB", CPUBogo: "108.00",
			Mem: Memory{Total: 3884296 * 1024, Available: 3100000 * 1024, Free: 2500000 * 1024, Buffers: 45000 * 1024, Cached: 520000 * 1024},
			Nets: []NetIface{
				{"lo", 1824, 1824},
				{"eth0", 12345678, 2345678},
				{"wlan0", 2345678, 123456},
			},
			FS: []Filesystem{
				{"/dev/mmcblk0p2", 30535680 * 1024, 8123456 * 1024, 20876544 * 1024, "/"},
				{"/dev/sda1", 976704112 * 1024, 123456789 * 1024, 803247323 * 1024, "/media/ssd"},
			},
		}},
		// CentOS 6: no os-release, two sockets with hyper-threading.
		{"sysinfo-centos.txt", SysInfo{
			Hostname: "db-legacy.example.com", OS: "CentOS release 6.10 (Final)", KernelOS: "Linux", Kernel: "2.6.32-754.35.1.el6.x86_64", Arch: "x86_64",
			CPUModel: "Intel(R) Xeon(R) CPU E5620 @ 2.40GHz", CPUs: 8, Cores: 4,
			MemTotal: 16330708 * 1024, SwapTotal: 8191996 * 1024, Uptime: 4021 * time.Second,
			Addrs: []Addr{{"eth0", "10.10.1.20/24"}, {"eth0", "10.10.1.21/24"}},
			Disks: []BlockDevice{{Name: "sda", Size: 209715200 * 512, Model: "Virtual disk"}, {Name: "sdb", Size: 1073741824 * 512, Model: "Virtual disk"}},

			Load: "0.42 0.55 0.61", UptimeUp: "up 1 hour, 7 minutes",
			IPs:     "10.10.1.20 10.10.1.21",
			CPUFreq: "2400.000", CPUCache: "12288 KB", CPUBogo: "4800.12",
			Mem: Memory{Total: 16330708 * 1024, Available: 15200000 * 1024, Free: 8000000 * 1024, Buffers: 234567 * 1024, Cached: 6000000 * 1024},
			Swap: Memory{Total: 8191996 * 1024, Available: 8091996 * 1024, Free: 8091996 * 1024},
			Nets: []NetIface{{"lo", 1824, 1824}, {"eth0", 543210987, 234567890}},
			FS: []Filesystem{
				{"/dev/sda1", 103212320 * 1024, 45678901 * 1024, 51234567 * 1024, "/"},
				{"/dev/sdb1", 524288000 * 1024, 12345678 * 1024, 511942322 * 1024, "/data"},
			},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			got := ParseSysInfo(readSample(t, tc.file))
			usage := got.Usage
			got.Usage = CPUUsage{} // divisions: summed below
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
			if sum := usage.User + usage.Nice + usage.System + usage.Idle + usage.IOWait + usage.IRQ + usage.SoftIRQ + usage.Steal; sum < 99.9 || sum > 100.1 {
				t.Errorf("%s: the usage sums to %v", tc.file, sum)
			}
		})
	}
}

// A server without most tools still gives what it has.
func TestParseSysInfoMissingTools(t *testing.T) {
	out := "---FINALLYSHELL:hostname---\nbox\n---FINALLYSHELL:uname---\nLinux\n5.10.0\nmips\n" +
		"---FINALLYSHELL:os-release---\n---FINALLYSHELL:release---\n---FINALLYSHELL:cpuinfo---\n" +
		"processor\t: 0\ncpu model\t: MIPS 24Kc V7.4\n---FINALLYSHELL:nproc---\n---FINALLYSHELL:addr---\n" +
		"---FINALLYSHELL:disks---\n---FINALLYSHELL:end---\n"
	got := ParseSysInfo(out)
	want := SysInfo{Hostname: "box", OS: "Linux", KernelOS: "Linux", Kernel: "5.10.0", Arch: "mips", CPUModel: "MIPS 24Kc V7.4", CPUs: 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if got := ParseSysInfo(""); !reflect.DeepEqual(got, SysInfo{}) {
		t.Errorf("empty output: %+v", got)
	}
}

func TestParseAddrsIfconfig(t *testing.T) {
	busybox := `eth0      Link encap:Ethernet  HWaddr 02:FC:00:00:00:01
          inet addr:192.0.2.2  Bcast:192.0.2.255  Mask:255.255.255.0
          inet6 addr: 2001:db8::2/64 Scope:Global
          inet6 addr: fe80::fc:ff:fe00:1/64 Scope:Link
          UP BROADCAST RUNNING MULTICAST  MTU:1400  Metric:1

lo        Link encap:Local Loopback
          inet addr:127.0.0.1  Mask:255.0.0.0
`
	netTools := `eth0: flags=4163<UP,BROADCAST,RUNNING,MULTICAST>  mtu 1400
        inet 192.0.2.2  netmask 255.255.255.0  broadcast 192.0.2.255
        inet6 2001:db8::2  prefixlen 64  scopeid 0x0<global>
        inet6 fe80::fc:ff:fe00:1  prefixlen 64  scopeid 0x20<link>
        ether 02:fc:00:00:00:01  txqueuelen 1000  (Ethernet)

lo: flags=73<UP,LOOPBACK,RUNNING>  mtu 65536
        inet 127.0.0.1  netmask 255.0.0.0
`
	want := []Addr{{"eth0", "192.0.2.2/24"}, {"eth0", "2001:db8::2/64"}}
	for name, s := range map[string]string{"busybox": busybox, "net-tools": netTools} {
		if got := parseAddrs(s); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := maskBits("0xffffff00"); got != 24 {
		t.Errorf("hex mask: %d", got)
	}
}

func TestSysInfoScriptSections(t *testing.T) {
	// Each section the parser reads is one the script writes.
	for _, name := range []string{"hostname", "uname", "os-release", "release", "cpuinfo", "lscpu", "nproc", "meminfo", "uptime",
		"uptext", "load", "ips", "stat", "freq", "cache", "bogo", "gpu", "netdev", "df", "addr", "disks", "end"} {
		if !strings.Contains(sysInfoScript, "m "+name+";") && !strings.Contains(sysInfoScript, "m "+name+"\n") {
			t.Errorf("the script has no %s section", name)
		}
	}
	if got := cleanIssue(`Ubuntu 24.04.5 LTS \n \l`); got != "Ubuntu 24.04.5 LTS" {
		t.Errorf("issue %q", got)
	}
}
