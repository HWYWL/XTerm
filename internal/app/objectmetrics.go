package app

import (
	"context"
	"sync"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/objectstore"
)

// objectMeter is what the object storage's System page shows: how the
// requests of the tab went, how much they carried, how far behind the
// server was, and what the bucket holds (objectmonitor.go).
type objectMeter struct {
	mu sync.Mutex

	// verbs counts the calls of each verb, their failures, and their
	// latencies, the newest last.
	verbs map[string]*objectVerb
	// codes counts the answers by their HTTP status; timeouts are the
	// requests that never got one.
	codes    map[int]int64
	timeouts int64
	ok       int64
	failed   int64

	// bytes counts what the tab read and wrote, and the day's totals,
	// which start again at midnight.
	inBytes, outBytes int64
	todayIn, todayOut int64
	day               time.Time
	lastIn, lastOut   int64
	lastAt            time.Time
	inRate, outRate   []float64
	qps               map[string][]float64
	lastCounts        map[string]int64
	lastSampleAt      time.Time
	// probe is the last look at the endpoint: when, how long it took, and
	// what it said.
	probeAt      time.Time
	probeLatency time.Duration
	probeState   string // "online", "denied", "offline" or "timeout"
	probeErr     error
}

// objectVerb is the calls of one verb.
type objectVerb struct {
	count   int64
	failed  int64
	latency []time.Duration
}

// objectVerbMax is how many latencies a verb keeps for its percentiles.
const objectVerbMax = 256

// initObjectMeter prepares the meter of a storage just connected.
func (m *objectMeter) init() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verbs = map[string]*objectVerb{}
	m.codes = map[int]int64{}
	m.qps = map[string][]float64{}
	m.lastCounts = map[string]int64{}
	m.inRate, m.outRate = nil, nil
	m.inBytes, m.outBytes, m.todayIn, m.todayOut = 0, 0, 0, 0
	m.lastIn, m.lastOut, m.lastAt = 0, 0, time.Time{}
	m.lastSampleAt = time.Time{}
	m.timeouts, m.ok, m.failed = 0, 0, 0
	m.probeAt, m.probeLatency, m.probeState, m.probeErr = time.Time{}, 0, "", nil
}

// noteObjectCall records a call of verb: how long it took, whether it
// worked, and the status the server gave. The bytes are the notes of their
// own, as they are known only once the body has been read or written.
func (s *Session) noteObjectCall(verb string, start time.Time, err error) {
	m := &s.objectMeter
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.verbs == nil {
		m.verbs, m.codes, m.qps, m.lastCounts = map[string]*objectVerb{}, map[int]int64{}, map[string][]float64{}, map[string]int64{}
	}
	v := m.verbs[verb]
	if v == nil {
		v = &objectVerb{}
		m.verbs[verb] = v
	}
	v.count++
	v.latency = append(v.latency, time.Since(start))
	if len(v.latency) > objectVerbMax {
		v.latency = v.latency[len(v.latency)-objectVerbMax:]
	}
	if err == nil {
		m.ok++
		m.codes[200]++
		return
	}
	v.failed++
	m.failed++
	if objectstore.IsTimeout(err) {
		m.timeouts++
		return
	}
	if status, ok := objectstore.HTTPStatus(err); ok {
		m.codes[status]++
	}
}

// noteObjectWritten records the bytes a Put wrote.
func (s *Session) noteObjectWritten(n int64) {
	m := &s.objectMeter
	m.mu.Lock()
	m.outBytes += n
	m.todayOut += n
	m.mu.Unlock()
}

// noteObjectRead records the bytes a Get read.
func (s *Session) noteObjectRead(n int64) {
	m := &s.objectMeter
	m.mu.Lock()
	m.inBytes += n
	m.todayIn += n
	m.mu.Unlock()
}

// noteObjectProbe records a look at the endpoint.
func (s *Session) noteObjectProbe(start time.Time, err error) {
	m := &s.objectMeter
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probeAt, m.probeLatency, m.probeErr = time.Now(), time.Since(start), err
	switch {
	case err == nil:
		m.probeState = "online"
	case objectstore.IsTimeout(err):
		m.probeState = "timeout"
	default:
		if status, ok := objectstore.HTTPStatus(err); ok && (status == 401 || status == 403) {
			m.probeState = "denied" // reachable, but the key may not look
			return
		}
		m.probeState = "offline"
	}
}

// sampleObjectMeter takes the rates and the QPS of the window since the
// sample before: the bytes' totals only grow, and so do the calls'.
func (s *Session) sampleObjectMeter(now time.Time) {
	m := &s.objectMeter
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.verbs == nil {
		return
	}
	if !m.day.Equal(now.Truncate(24 * time.Hour)) {
		m.day, m.todayIn, m.todayOut = now.Truncate(24*time.Hour), 0, 0
	}
	if !m.lastAt.IsZero() {
		if seconds := now.Sub(m.lastAt).Seconds(); seconds > 0 {
			in := float64(m.inBytes-m.lastIn) / seconds
			out := float64(m.outBytes-m.lastOut) / seconds
			if in < 0 {
				in = 0 // the totals start again, as a restarted server's do
			}
			if out < 0 {
				out = 0
			}
			m.inRate = objectHist(m.inRate, in)
			m.outRate = objectHist(m.outRate, out)
		}
	}
	m.lastIn, m.lastOut, m.lastAt = m.inBytes, m.outBytes, now
	if m.lastSampleAt.IsZero() {
		m.lastSampleAt = now
	}
	seconds := now.Sub(m.lastSampleAt).Seconds()
	if seconds <= 0 {
		return
	}
	for verb, v := range m.verbs {
		delta := v.count - m.lastCounts[verb]
		m.lastCounts[verb] = v.count
		m.qps[verb] = objectHist(m.qps[verb], float64(delta)/seconds)
	}
	m.lastSampleAt = now
}

// objectHist adds v to h, keeping the samples the curves show.
func objectHist(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > monitor.HistoryLen {
		h = h[len(h)-monitor.HistoryLen:]
	}
	return h
}

// objectSnapshot is what the page shows, copied under the lock.
type objectSnapshot struct {
	verbs      map[string]*objectVerb
	codes      map[int]int64
	timeouts   int64
	ok, failed int64
	in, out    int64
	todayIn    int64
	todayOut   int64
	inRate     []float64
	outRate    []float64
	qps        map[string][]float64
	probeAt    time.Time
	probeLat   time.Duration
	probeState string
	probeErr   error
}

// count is the calls of a verb.
func (s objectSnapshot) count(verb string) int64 {
	if v := s.verbs[verb]; v != nil {
		return v.count
	}
	return 0
}

// latency is the latencies of a verb's calls, the newest last.
func (s objectSnapshot) latency(verb string) []time.Duration {
	if v := s.verbs[verb]; v != nil {
		return v.latency
	}
	return nil
}

// snapshot copies the meter for the page to draw.
func (m *objectMeter) snapshot() objectSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	verbs := make(map[string]*objectVerb, len(m.verbs))
	for name, v := range m.verbs {
		verbs[name] = &objectVerb{count: v.count, failed: v.failed, latency: append([]time.Duration(nil), v.latency...)}
	}
	codes := make(map[int]int64, len(m.codes))
	for code, n := range m.codes {
		codes[code] = n
	}
	qps := make(map[string][]float64, len(m.qps))
	for name, h := range m.qps {
		qps[name] = append([]float64(nil), h...)
	}
	s := objectSnapshot{
		verbs: verbs, codes: codes, timeouts: m.timeouts, ok: m.ok, failed: m.failed,
		in: m.inBytes, out: m.outBytes, todayIn: m.todayIn, todayOut: m.todayOut,
		inRate: append([]float64(nil), m.inRate...), outRate: append([]float64(nil), m.outRate...),
		qps:     qps,
		probeAt: m.probeAt, probeLat: m.probeLatency, probeState: m.probeState, probeErr: m.probeErr,
	}
	return s
}

// startObjectMonitor samples the meter every second while the session is
// open, and probes the endpoint every ten.
func (a *App) startObjectMonitor(s *Session, ctx context.Context, store objectstore.Store) {
	s.objectMeter.init()
	go func() {
		sample := time.NewTicker(time.Second)
		defer sample.Stop()
		probe := time.NewTicker(10 * time.Second)
		defer probe.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-sample.C:
				a.post(func() {
					if ctx.Err() == nil {
						s.sampleObjectMeter(now)
					}
				})
			case <-probe.C:
				a.probeObject(s, ctx, store)
			}
		}
	}()
	a.probeObject(s, ctx, store)
}

// probeObject looks at the endpoint, and records what it took.
func (a *App) probeObject(s *Session, ctx context.Context, store objectstore.Store) {
	go func() {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		start := time.Now()
		err := store.Probe(pctx)
		a.post(func() {
			if ctx.Err() == nil {
				s.noteObjectProbe(start, err)
			}
		})
	}()
}
