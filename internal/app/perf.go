package app

import (
	"fmt"
	"runtime"
	"runtime/metrics"
	"strconv"
	"time"

	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The performance window: what the app itself costs, as QuickTerm's panel
// shows for its runtime: the goroutines, the heap, the garbage collections
// and their pauses, and the CPU time spent, sampled a second at a time while
// the window is open.

// processStart is where the running time shown counts from.
var processStart = time.Now()

// perfSamples is how many seconds of samples the trends keep.
const perfSamples = 60

// perfSample is one second: the goroutines, the heap in bytes, the CPU
// seconds spent so far, the collections and the pause of the last, and the
// heap size that starts the next one.
type perfSample struct {
	goroutines int
	heap, sys  float64
	nextGC     float64
	cpu        float64
	gc         uint32
	pause      time.Duration
}

// perfState is the samples, newest last, on the UI thread alone.
type perfState struct {
	samples []perfSample
}

// perfWindow is its spec: as tall as the numbers and the two trends, not
// taller, so that the window has no empty tail under them.
var perfWindow = WindowSpec{
	Kind:      WindowPerf,
	Title:     locales.PerfTitle,
	Width:     820,
	Height:    410,
	MinWidth:  660,
	MinHeight: 400,
}

// openPerf opens the window, sampling from then on while it is open.
func (a *App) openPerf() {
	if !a.windowOpen(WindowPerf) {
		a.perf.samples = nil
		a.samplePerf()
		go a.perfLoop()
	}
	a.openWindow(perfWindow, a.perfView)
}

// perfLoop samples a second at a time, on the UI thread through post, until
// the window closes.
func (a *App) perfLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		done := make(chan bool, 1)
		a.post(func() {
			if !a.windowOpen(WindowPerf) {
				done <- true
				return
			}
			a.samplePerf()
			a.redraw()
			done <- false
		})
		if <-done {
			return
		}
	}
}

// samplePerf takes one sample of the runtime: the window calls it as it
// opens, and perfLoop every second after.
func (a *App) samplePerf() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	cpu := 0.0
	samples := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}}
	metrics.Read(samples)
	if samples[0].Value.Kind() == metrics.KindFloat64 {
		cpu = samples[0].Value.Float64()
	}
	s := perfSample{
		goroutines: runtime.NumGoroutine(),
		heap:       float64(m.HeapAlloc),
		sys:        float64(m.Sys),
		nextGC:     float64(m.NextGC),
		cpu:        cpu,
		gc:         m.NumGC,
		pause:      time.Duration(m.PauseNs[(m.NumGC+255)%256]),
	}
	a.perf.samples = append(a.perf.samples, s)
	if len(a.perf.samples) > perfSamples {
		a.perf.samples = a.perf.samples[1:]
	}
}

// perfCols is how many numbers a row has: the rows keep their cards the same
// width, the last one filled with an empty cell where it is short.
const perfCols = 4

// perfStat is one card of the numbers: its label, its value, the hint under it
// where there is one, and the colour the value takes.
type perfStat struct {
	label, value, hint string
	color              ui.Color
}

// perfView is the window: the numbers of the last sample, then the trends.
func (a *App) perfView(c *ui.Context) {
	if len(a.perf.samples) == 0 {
		a.samplePerf()
	}
	last := a.perf.samples[len(a.perf.samples)-1]
	stats := []perfStat{
		{locales.PerfGoroutines(), strconv.Itoa(last.goroutines), locales.PerfGoroutinesHint(), assetCyan},
		{locales.PerfHeap(), humanSize(int64(last.heap)), locales.PerfHeapHint(), assetBlue},
		{locales.PerfSys(), humanSize(int64(last.sys)), locales.PerfSysHint(), assetPurple},
		{locales.PerfCPU(), fmt.Sprintf("%.1f s", last.cpu), locales.PerfCPUHint(), assetOrange},
		{locales.PerfGC(), strconv.FormatUint(uint64(last.gc), 10), locales.PerfGCHint(), assetGreen},
		{locales.PerfNextGC(), humanSize(int64(last.nextGC)), locales.PerfNextGCHint(), assetGreen},
		{locales.PerfPause(), last.pause.Round(time.Microsecond).String(), locales.PerfPauseHint(), assetGreen},
		{locales.PerfUptime(), time.Since(processStart).Round(time.Second).String(), locales.PerfUptimeHint(), theme.TextMuted},
	}
	ui.Column(c).Fill().Padding(12, 14, 12, 14).Gap(8).Children(func() {
		ui.Text(c, locales.PerfNote()).FontSize(11).TextColor(theme.TextMuted).MaxLines(2)
		for i := 0; i < len(stats); i += perfCols {
			row := stats[i:min(i+perfCols, len(stats))]
			ui.Row(c).Gap(8).Children(func() {
				for _, s := range row {
					statCard(c, s.label, s.value, s.hint, s.color)
				}
				for j := len(row); j < perfCols; j++ {
					ui.Box(c).Basis(0).Grow(1).Shrink(1).MinWidth(0)
				}
			})
		}
		heap, gors := make([]float64, 0, len(a.perf.samples)), make([]float64, 0, len(a.perf.samples))
		for _, s := range a.perf.samples {
			heap = append(heap, s.heap)
			gors = append(gors, float64(s.goroutines))
		}
		ui.Text(c, locales.PerfHeapTrend()).FontSize(11).TextColor(theme.TextMuted)
		widgets.Sparkline(c, perfSamples, [][]float64{heap}, []ui.Color{assetBlue},
			func(v float64) string { return humanSize(int64(v)) })
		ui.Text(c, locales.PerfGoroutineTrend()).FontSize(11).TextColor(theme.TextMuted)
		widgets.Sparkline(c, perfSamples, [][]float64{gors}, []ui.Color{assetCyan},
			func(v float64) string { return strconv.Itoa(int(v)) })
	})
}
