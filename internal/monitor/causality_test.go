package monitor

import (
	"context"
	"strings"
	"testing"
)

// The target 1200 runs as root under init, which adopted it when its
// parent exited.
const causalitySample = "FS_NODE\t1200\t900\troot\tS\t3600\t20480\torphan\t/usr/bin/mydaemon --serve\n" +
	"FS_NODE\t900\t1\troot\tS\t90000\t1024\tinit\t/sbin/init\n" +
	"FS_FACT\tcwd\t/var/lib/mydaemon\n" +
	"FS_FACT\texe\t/usr/bin/mydaemon\n" +
	"FS_WARN\tdanger\troot\n" +
	"FS_WARN\twarn\tbig-rss\t20480\n" +
	"FS_WARN\tinfo\tssh\n" +
	"FS_META\t0\n" +
	"FS_END\n"

// TestParseCausality reads the chain from the root down, with the facts of
// the target and the warnings the script found.
func TestParseCausality(t *testing.T) {
	c := parseCausality(1200, causalitySample)
	if !c.Found || c.Degraded || c.PID != 1200 {
		t.Fatalf("%+v", c)
	}
	if len(c.Nodes) != 2 {
		t.Fatalf("%d nodes: %+v", len(c.Nodes), c.Nodes)
	}
	root, target := c.Nodes[0], c.Nodes[1]
	if root.PID != 900 || root.IsTarget || root.Kind != "init" || root.Command != "/sbin/init" {
		t.Errorf("root %+v", root)
	}
	if target.PID != 1200 || !target.IsTarget || target.PPID != 900 || target.State != 'S' ||
		target.Age != 3600 || target.RSS != 20480*1024 || target.Kind != "orphan" ||
		target.Command != "/usr/bin/mydaemon --serve" {
		t.Errorf("target %+v", target)
	}
	want := []CausalityWarning{
		{Level: "danger", Code: "root"},
		{Level: "warn", Code: "big-rss", Arg: "20480"},
		{Level: "info", Code: "ssh"},
	}
	if len(c.Warnings) != len(want) {
		t.Fatalf("warnings %+v", c.Warnings)
	}
	for i := range want {
		if c.Warnings[i] != want[i] {
			t.Errorf("warning %d: %+v", i, c.Warnings[i])
		}
	}
}

// A chain the server could not read says why, and a node line the script
// cut short is left out rather than guessed at.
func TestParseCausalityReasons(t *testing.T) {
	gone := parseCausality(42, "FS_REASON\tgone\nFS_END\n")
	if gone.Found || gone.Reason != "gone" {
		t.Errorf("%+v", gone)
	}
	short := parseCausality(42, "FS_NODE\t42\t1\tnobody\nFS_END\n")
	if short.Found || len(short.Nodes) != 0 {
		t.Errorf("%+v", short)
	}
	degraded := parseCausality(42, "FS_NODE\t42\t1\troot\tR\t-\t-\tproc\tx y\tz\nFS_META\t1\nFS_END\n")
	if !degraded.Found || !degraded.Degraded || degraded.Nodes[0].Command != "x y\tz" {
		t.Errorf("%+v", degraded)
	}
	if degraded.Nodes[0].Age != 0 || degraded.Nodes[0].RSS != 0 {
		t.Errorf("unknown age or memory: %+v", degraded.Nodes[0])
	}
}

// The script carries the PID it walks, and asks /proc alone.
func TestCausalityScript(t *testing.T) {
	s := causalityScript(4321)
	if !strings.Contains(s, "P=4321") || strings.Contains(s, "{pid}") {
		t.Errorf("script %q", s)
	}
	if !strings.Contains(s, `$PROC/$1/stat`) || strings.Contains(s, "ps -") {
		t.Errorf("script reads more than /proc: %q", s)
	}
}

// A PID no process can have is no process, with no round trip.
func TestReadCausalityBadPID(t *testing.T) {
	c, err := ReadCausality(context.Background(), nil, 0)
	if err != nil || c.Found || c.Reason != "gone" {
		t.Errorf("%+v, %v", c, err)
	}
}
