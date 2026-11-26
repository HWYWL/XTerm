package app

import (
	"testing"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/redis"
)

// The panel's curves come from INFO's counters: the commands as the server
// gives them, and the traffic from the totals over the time between two
// samples, so that a counter that starts over is no spike.
func TestNoteRedisStats(t *testing.T) {
	s := &Session{}
	s.noteRedisStats(redis.ParseInfo("# Stats\ninstantaneous_ops_per_sec:10\n"))
	if len(s.redisOpsHist) != 1 || s.redisOpsHist[0] != 10 {
		t.Fatalf("ops %v", s.redisOpsHist)
	}
	if len(s.redisInHist) != 0 {
		t.Errorf("a rate from a single sample: %v", s.redisInHist)
	}

	// A second sample a second later: 1000 bytes in and 500 out are the
	// rates of it.
	s.redisNetAt = time.Now().Add(-time.Second)
	s.noteRedisStats(redis.ParseInfo("# Stats\ninstantaneous_ops_per_sec:20\ntotal_net_input_bytes:1000\ntotal_net_output_bytes:500\n"))
	if len(s.redisInHist) != 1 || len(s.redisOutHist) != 1 {
		t.Fatalf("rates in %v, out %v", s.redisInHist, s.redisOutHist)
	}
	if got := s.redisInHist[0]; got < 500 || got > 1500 {
		t.Errorf("in %v, want about 1000 a second", got)
	}
	if got := s.redisOutHist[0]; got < 250 || got > 750 {
		t.Errorf("out %v, want about 500 a second", got)
	}

	// A counter that went back, as a restarted server's, is no negative
	// rate.
	s.redisNetAt = time.Now().Add(-time.Second)
	s.noteRedisStats(redis.ParseInfo("# Stats\ntotal_net_input_bytes:10\ntotal_net_output_bytes:5\n"))
	if got := s.redisInHist[len(s.redisInHist)-1]; got != 0 {
		t.Errorf("a counter that started over: %v", got)
	}

	// The curves keep the samples of the window, not more.
	for i := 0; i < 2*monitor.HistoryLen; i++ {
		s.noteRedisStats(redis.ParseInfo("# Stats\ninstantaneous_ops_per_sec:1\n"))
	}
	if len(s.redisOpsHist) != monitor.HistoryLen {
		t.Errorf("%d samples, want %d", len(s.redisOpsHist), monitor.HistoryLen)
	}
}
