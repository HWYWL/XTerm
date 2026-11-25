package monitor

import (
	"bufio"
	"math"
	"strconv"
	"strings"
	"time"
)

// The stats of a Mac, which has no /proc: sysctl tells the boot time, the
// load and the memory, top the CPU, vm_stat the memory in use, df the
// disks and netstat the interfaces, in the shape /proc/net/dev has them.
// The sensors are left out: a Mac's are not in /sys.

// statsScriptDarwin reads everything in one round trip, its parts in the
// order parseDarwin reads them.
const statsScriptDarwin = `export LC_ALL=C
sysctl -n kern.boottime; echo ` + sep + `
sysctl -n vm.loadavg; echo ` + sep + `
top -l 1 -n 0 2>/dev/null | grep 'CPU usage'; echo ` + sep + `
vm_stat; echo ` + sep + `
sysctl -n hw.memsize; echo ` + sep + `
sysctl -n vm.swapusage; echo ` + sep + `
df -kP; echo ` + sep + `
netstat -ibn; echo ` + sep + `
ps -ax -o pid= 2>/dev/null | wc -l; echo ` + sep + `
who 2>/dev/null | wc -l
`

// parseDarwin reads the output of statsScriptDarwin.
func parseDarwin(st *Stats, parts []string, net *netState, now time.Time) {
	st.Uptime = parseBootUptime(parts[0], now)
	st.Load = parseLoadValues(parts[1])
	st.CPU = parseDarwinCPU(parts[2])
	st.MemUsed = parseVMStatUsed(parts[3])
	if total, err := strconv.ParseUint(strings.TrimSpace(parts[4]), 10, 64); err == nil {
		st.MemTotal = total
	}
	if st.MemTotal > 0 && st.MemUsed > st.MemTotal {
		st.MemUsed = st.MemTotal
	}
	if len(parts) > 5 {
		st.SwapTotal, st.SwapUsed = parseSwapUsage(parts[5])
	}
	if len(parts) > 6 {
		st.Disks = parseDisks(parts[6])
	}
	if len(parts) > 7 {
		net.updateCounters(st, parseDarwinNetstat(parts[7]), nil, now)
	}
	if len(parts) > 8 {
		st.ProcTotal = parseCount(parts[8])
	}
	if len(parts) > 9 {
		st.Users = parseCount(parts[9])
	}
}

// parseDarwinNetstat reads netstat -ibn's counters: each interface's
// bytes once, from its first line, which has its address; a line whose
// address is empty is one field shorter, and the header's width tells.
func parseDarwinNetstat(s string) []netCounters {
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return nil
	}
	wide := len(strings.Fields(lines[0])) // 11 with Coll, 10 without
	if wide < 9 {
		wide = 11
	}
	seen := map[string]bool{}
	var out []netCounters
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		name := f[0]
		if isLoopback(name) || seen[name] {
			continue
		}
		shift := wide - len(f)
		if shift < 0 || shift > 1 {
			continue
		}
		ib, ob := 6-shift, 9-shift
		if ob >= len(f) {
			continue
		}
		rx, err1 := strconv.ParseUint(f[ib], 10, 64)
		tx, err2 := strconv.ParseUint(f[ob], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		seen[name] = true
		out = append(out, netCounters{name: name, rx: rx, tx: tx})
	}
	return out
}

// parseBootUptime reads kern.boottime, "{ sec = 1690000000, usec = 0 }",
// as the time since then.
func parseBootUptime(s string, now time.Time) time.Duration {
	i := strings.Index(s, "sec = ")
	if i < 0 {
		return 0
	}
	f := strings.Fields(s[i+len("sec = "):])
	if len(f) == 0 {
		return 0
	}
	sec, err := strconv.ParseInt(strings.TrimSuffix(f[0], ","), 10, 64)
	if err != nil || sec <= 0 {
		return 0
	}
	return now.Sub(time.Unix(sec, 0))
}

// parseLoadValues reads vm.loadavg, "{ 1.20 1.30 1.10 }".
func parseLoadValues(s string) (l [3]float64) {
	f := strings.Fields(strings.Trim(s, "{} \t\r\n"))
	for i := 0; i < 3 && i < len(f); i++ {
		l[i], _ = strconv.ParseFloat(f[i], 64)
	}
	return l
}

// parseDarwinCPU reads top's "CPU usage: 3.12% user, 5.20% sys, 91.67%
// idle" as the percent in use.
func parseDarwinCPU(s string) float64 {
	i := strings.Index(s, "idle")
	if i < 0 {
		return 0
	}
	head := s[:i]
	j := strings.LastIndex(head, "%")
	if j < 0 {
		return 0
	}
	k := j
	for k > 0 && (head[k-1] >= '0' && head[k-1] <= '9' || head[k-1] == '.') {
		k--
	}
	idle, err := strconv.ParseFloat(head[k:j], 64)
	if err != nil {
		return 0
	}
	return math.Max(0, 100-idle)
}

// parseVMStatUsed reads vm_stat as the bytes in use: the active and wired
// pages and those in the compressor, which is what a Mac counts as used;
// the free, inactive and speculative ones may be had back.
func parseVMStatUsed(s string) uint64 {
	const pagePrefix = "page size of "
	page := uint64(4096)
	vals := map[string]uint64{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, pagePrefix); i >= 0 {
			f := strings.Fields(line[i+len(pagePrefix):])
			if len(f) > 0 {
				if n, err := strconv.ParseUint(f[0], 10, 64); err == nil && n > 0 {
					page = n
				}
			}
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.Trim(strings.TrimSpace(name), `"`)
		n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(rest), "."), 10, 64)
		if err != nil {
			continue
		}
		vals[name] = n
	}
	pages := vals["Pages active"] + vals["Pages wired down"] + vals["Pages occupied by compressor"]
	return pages * page
}

// parseSwapUsage reads vm.swapusage, "total = 2048.00M  used = 100.00M
// free = 1948.00M", in bytes.
func parseSwapUsage(s string) (total, used uint64) {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '=' || r == '\t' })
	for i := 0; i+1 < len(f); i += 2 {
		switch f[i] {
		case "total":
			total = parseHumanBytes(f[i+1])
		case "used":
			used = parseHumanBytes(f[i+1])
		}
	}
	return total, used
}

// parseHumanBytes reads a size as vm.swapusage prints it, 2048.00M.
func parseHumanBytes(s string) uint64 {
	if s == "" {
		return 0
	}
	mult := 1.0
	switch s[len(s)-1] {
	case 'K', 'k':
		mult, s = 1024, s[:len(s)-1]
	case 'M', 'm':
		mult, s = 1024*1024, s[:len(s)-1]
	case 'G', 'g':
		mult, s = 1024*1024*1024, s[:len(s)-1]
	case 'B', 'b':
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return uint64(v * mult)
}
