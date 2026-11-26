package app

import (
	"context"
	"testing"
	"time"

	"finallyshell/internal/objectstore"
)

// The meter counts the calls of each verb by their outcome, their status
// and their time; the bytes are the notes of their own; and the samples
// turn the totals into rates and the counts into a QPS.
func TestObjectMetrics(t *testing.T) {
	s := &Session{}
	s.objectMeter.init()
	s.noteObjectCall("Get", time.Now().Add(-40*time.Millisecond), nil)
	s.noteObjectCall("Put", time.Now().Add(-10*time.Millisecond), &objectstore.StatusError{Status: 403})
	s.noteObjectCall("List", time.Now().Add(-5*time.Millisecond), context_deadline())
	s.noteObjectRead(1000)
	s.noteObjectWritten(500)

	m := s.objectMeter.snapshot()
	if m.ok != 1 || m.failed != 2 {
		t.Errorf("ok %d, failed %d", m.ok, m.failed)
	}
	if m.codes[200] != 1 || m.codes[403] != 1 {
		t.Errorf("codes %v", m.codes)
	}
	if m.timeouts != 1 {
		t.Errorf("timeouts %d", m.timeouts)
	}
	if m.count("Get") != 1 || m.count("Put") != 1 || m.count("List") != 1 {
		t.Errorf("counts %d %d %d", m.count("Get"), m.count("Put"), m.count("List"))
	}
	if p50, _, _ := objectPercentiles(m.latency("Get")); p50 < 20*time.Millisecond {
		t.Errorf("the Get p50 %v", p50)
	}
	if m.in != 1000 || m.out != 500 {
		t.Errorf("bytes in %d out %d", m.in, m.out)
	}
	if m.todayIn != 1000 || m.todayOut != 500 {
		t.Errorf("today in %d out %d", m.todayIn, m.todayOut)
	}

	// Two samples two seconds apart: a thousand bytes read is 500 a
	// second, and the Get is a call a second.
	now := time.Now()
	s.objectMeter.mu.Lock()
	s.objectMeter.lastAt, s.objectMeter.lastSampleAt = now, now
	s.objectMeter.lastIn, s.objectMeter.lastOut = 0, 500
	s.objectMeter.lastCounts["Get"] = 0
	s.objectMeter.mu.Unlock()
	s.noteObjectRead(1000)
	s.objectMeter.mu.Lock()
	s.objectMeter.inBytes += 0
	s.objectMeter.mu.Unlock()
	s.sampleObjectMeter(now.Add(2 * time.Second))
	m = s.objectMeter.snapshot()
	if len(m.inRate) == 0 || m.inRate[len(m.inRate)-1] < 900 || m.inRate[len(m.inRate)-1] > 1100 {
		t.Errorf("the read rate %v", m.inRate)
	}
	if got := m.qps["Get"]; len(got) == 0 || got[len(got)-1] < 0.4 || got[len(got)-1] > 0.6 {
		t.Errorf("the Get qps %v", got)
	}

	// A meter that starts again does not draw a negative rate.
	s.noteObjectRead(0)
	s.objectMeter.mu.Lock()
	s.objectMeter.inBytes = 10
	s.objectMeter.mu.Unlock()
	s.sampleObjectMeter(now.Add(4 * time.Second))
	m = s.objectMeter.snapshot()
	if last := m.inRate[len(m.inRate)-1]; last < 0 {
		t.Errorf("a rate below zero: %v", last)
	}
}

// The percentiles of no calls are nothing, not a division by zero.
func TestObjectPercentilesEmpty(t *testing.T) {
	if p50, p90, p99 := objectPercentiles(nil); p50 != 0 || p90 != 0 || p99 != 0 {
		t.Errorf("%v %v %v", p50, p90, p99)
	}
}

// context_deadline is a request that ran out of time, as a timeout's.
func context_deadline() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	return ctx.Err()
}
