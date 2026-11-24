package app

import (
	"math"
	"testing"

	"finallyshell/locales"
)

// The performance window shows the app's own numbers, sampled as it opens
// and every second after.
func TestPerfWindow(t *testing.T) {
	a, p := testApp(t)
	a.openPlugin(pluginPerf)
	w := p.show(t, WindowPerf)
	w.Frame()
	for _, s := range []string{
		locales.PerfNote(), locales.PerfGoroutines(), locales.PerfHeap(), locales.PerfSys(),
		locales.PerfCPU(), locales.PerfGC(), locales.PerfNextGC(), locales.PerfPause(), locales.PerfUptime(),
		locales.PerfHeapTrend(), locales.PerfGoroutineTrend(),
	} {
		if !w.HasText(s) {
			t.Errorf("no %q; texts %q", s, w.Texts())
		}
	}
	if len(a.perf.samples) != 1 {
		t.Fatalf("samples %d", len(a.perf.samples))
	}

	// The trend keeps the last minute, and no more.
	for i := 0; i < perfSamples+5; i++ {
		a.samplePerf()
	}
	if len(a.perf.samples) != perfSamples {
		t.Errorf("samples %d", len(a.perf.samples))
	}
	w.Frame()
	if !w.HasText(locales.PerfGoroutineTrend()) {
		t.Errorf("the window went; texts %q", w.Texts())
	}
}

// The numbers keep their columns: each card starts where the one over it
// does, and the second row is as full as the first.
func TestPerfColumnsLineUp(t *testing.T) {
	a, p := testApp(t)
	a.openPlugin(pluginPerf)
	w := p.show(t, WindowPerf)
	w.Frame()
	labels := []string{
		locales.PerfGoroutines(), locales.PerfHeap(), locales.PerfSys(), locales.PerfCPU(),
		locales.PerfGC(), locales.PerfNextGC(), locales.PerfPause(), locales.PerfUptime(),
	}
	var xs, ys []float32
	for _, l := range labels {
		r, ok := w.Find(l)
		if !ok {
			t.Fatalf("no %q; texts %q", l, w.Texts())
		}
		xs, ys = append(xs, r.X), append(ys, r.Y)
	}
	for i := 4; i < len(labels); i++ {
		if math.Abs(float64(xs[i]-xs[i-4])) > 0.5 {
			t.Errorf("%s at x %v, %s over it at %v", labels[i], xs[i], labels[i-4], xs[i-4])
		}
	}
	if math.Abs(float64(ys[3]-ys[0])) > 0.5 || math.Abs(float64(ys[7]-ys[4])) > 0.5 || ys[3] >= ys[4] {
		t.Errorf("the rows at y %v and %v", ys[:4], ys[4:])
	}
}
