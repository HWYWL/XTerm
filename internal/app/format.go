package app

import (
	"fmt"
	"time"
)

// formatSize is a file's size: 155.6 KB.
func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		f /= unit
		if f < unit {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return fmt.Sprintf("%.1f PB", f/unit)
}

// formatBytes is a short size for the side panel: 2.7G, 113M, 65K.
func formatBytes(n float64) string {
	units := []string{"B", "K", "M", "G", "T"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if i == 0 || n >= 100 {
		return fmt.Sprintf("%.0f%s", n, units[i])
	}
	return fmt.Sprintf("%.1f%s", n, units[i])
}

// formatRate is a speed, as the network card shows it: 2.8 K/s, 43.9 K/s.
func formatRate(n float64) string {
	s := formatBytes(n)
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i == len(s) {
		return s + "/s"
	}
	return s[:i] + " " + s[i:] + "/s"
}

func formatLatency(d time.Duration) string {
	ms := float64(d.Microseconds()) / 1000
	if ms < 10 {
		return fmt.Sprintf("%.1f ms", ms)
	}
	return fmt.Sprintf("%.0f ms", ms)
}
