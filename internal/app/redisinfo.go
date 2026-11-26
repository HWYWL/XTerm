package app

import (
	"context"
	"time"

	"finallyshell/internal/monitor"
	"finallyshell/internal/redis"
)

// redisInfoPoll is how often a Redis tab's System page asks the server what
// it says of itself; tests shorten it.
var redisInfoPoll = 2 * time.Second

// startRedisInfo reads INFO on c, the panel's connection of its own, and
// keeps the answer in s, until ctx ends.
func (a *App) startRedisInfo(s *Session, ctx context.Context, c *redis.Client) {
	go func() {
		for {
			rctx, cancel := context.WithTimeout(ctx, redisTimeout)
			info, err := c.Info(rctx)
			cancel()
			if ctx.Err() != nil {
				return
			}
			a.post(func() {
				if ctx.Err() != nil || s.redisStats != c {
					return
				}
				s.redisInfo, s.redisInfoErr, s.redisInfoAt = info, err, time.Now()
				if err == nil {
					s.noteRedisStats(info)
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(redisInfoPoll):
			}
		}
	}()
}

// noteRedisStats keeps what the panel draws as curves: the commands per
// second, and the bytes in and out — from the totals, which only grow, and
// the time since the sample before.
func (s *Session) noteRedisStats(in redis.Info) {
	now := time.Now()
	s.redisOpsHist = redisHist(s.redisOpsHist, float64(max(in.Int("Stats", "instantaneous_ops_per_sec"), 0)))
	inB := uint64(max(in.Int("Stats", "total_net_input_bytes"), 0))
	outB := uint64(max(in.Int("Stats", "total_net_output_bytes"), 0))
	if !s.redisNetAt.IsZero() {
		if seconds := now.Sub(s.redisNetAt).Seconds(); seconds > 0 {
			var in, out float64
			if inB >= s.redisInBytes {
				in = float64(inB-s.redisInBytes) / seconds
			}
			if outB >= s.redisOutBytes {
				out = float64(outB-s.redisOutBytes) / seconds
			}
			s.redisInHist = redisHist(s.redisInHist, in)
			s.redisOutHist = redisHist(s.redisOutHist, out)
		}
	}
	s.redisInBytes, s.redisOutBytes, s.redisNetAt = inB, outB, now
}

// redisHist adds v to h, keeping the samples the curves show.
func redisHist(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > monitor.HistoryLen {
		h = h[len(h)-monitor.HistoryLen:]
	}
	return h
}

// redisSectionShown is the INFO section the panel shows: the one chosen
// while the server still sends it, else Server, else the first.
func (a *App) redisSectionShown(in redis.Info) string {
	if a.redisSection != "" && in.Section(a.redisSection) != nil {
		return a.redisSection
	}
	if in.Section("Server") != nil {
		return "Server"
	}
	if len(in.Sections) > 0 {
		return in.Sections[0].Name
	}
	return ""
}
