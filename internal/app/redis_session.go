package app

import (
	"context"
	"net"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/redis"
	"finallyshell/internal/sshx"

	"github.com/egoist/mygo/plugins/terminal"
	"golang.org/x/crypto/ssh"
)

// redisDialTimeout is how long opening one connection to a node waits, as
// the proxy or jump host on the way may not answer.
const redisDialTimeout = 10 * time.Second

// redisOptions are how h is reached, with secret as its password: the
// address of a standalone server, or the seeds of a cluster.
func redisOptions(h *config.Host, secret string) redis.Options {
	o := redis.Options{Password: secret}
	if h.Redis != nil && h.Redis.Cluster {
		o.Cluster, o.Seeds = true, h.RedisSeeds()
		return o
	}
	o.Addr = h.AddrPort(6379)
	if h.Redis != nil {
		o.DB = h.Redis.DB
	}
	return o
}

// connectRedis connects s to its Redis server: one connection for each pane
// of the layout, whose command line the pane shows as a terminal's. The tab
// has no shell, no files and no monitor: the protocol has none of them.
func (a *App) connectRedis(s *Session, secret string) {
	if s.cancel != nil {
		s.cancel()
	}
	s.status, s.err, s.hostKeyQ = statusConnecting, nil, nil
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.ctx = cancel, ctx
	h := *s.Host
	s.redisOpts = redisOptions(&h, secret)
	// Through its proxy or jump hosts, if it has any: the connection to
	// each node is of its own, as a cluster moves them (sshx.DialTCP).
	jump := false
	var route sshx.Options
	if p := config.ProxyOf(&h); p != nil {
		opts, err := a.dialOptions(&h, secret, func(q *sshx.HostKeyQuestion) { a.post(func() { s.hostKeyQ = q }) })
		if err != nil {
			s.status, s.err = statusFailed, err
			return
		}
		route = opts
		route.KeepAlive = a.keepAlive()
		s.redisOpts.Dial = func(ctx context.Context, addr string) (net.Conn, error) {
			return sshx.DialTCP(&h, route, addr, redisDialTimeout)
		}
		jump = p.Kind == config.ProxyJump
	}
	spec := a.shellSpec(&h) // here, on the UI thread
	if s.panes.root == nil {
		s.panes.reset(newPane())
	}
	panes := s.panes.panes()
	fail := func(err error) {
		a.post(func() {
			if ctx.Err() != nil {
				return
			}
			s.status, s.err = statusFailed, err
			a.assetLogin(false)
		})
	}
	go func() {
		terms := make([]*terminal.Terminal, len(panes))
		clis := make([]*redisCLI, len(panes))
		for i, p := range panes {
			client, err := redis.Dial(ctx, s.redisOpts)
			if err != nil {
				for _, t := range terms {
					if t != nil {
						t.Close()
					}
				}
				fail(err)
				return
			}
			term, cli, err := a.redisTerminal(s, p, client, spec.topts, s.redisOpts.DB)
			if err != nil {
				client.Close()
				for _, t := range terms {
					if t != nil {
						t.Close()
					}
				}
				fail(err)
				return
			}
			terms[i], clis[i] = term, cli
		}
		// The jump host's own connection carries the tunnels: they are
		// its forwards, which the server behind it does not make.
		var jc *ssh.Client
		if jump {
			jc, _ = sshx.JumpClient(route)
		}
		a.post(func() {
			if ctx.Err() != nil {
				for _, t := range terms {
					if t != nil {
						t.Close()
					}
				}
				if jc != nil {
					jc.Close()
				}
				return
			}
			for i, p := range panes {
				switch {
				case terms[i] == nil:
					s.panes.remove(p)
					continue
				case !s.panes.has(p): // closed while connecting
					terms[i].Close()
					continue
				}
				if p.term != nil {
					p.term.Close()
				}
				p.term, p.redis = terms[i], clis[i]
				s.panes.opened(p)
				a.updateTerminal(p.term, spec.topts)
			}
			s.panes.refocus = true
			s.status, s.err = statusConnected, nil
			a.assetLogin(true)
			a.noteConnected(s.Host)
			if jc != nil {
				s.client = jc
				a.startTunnels(s, h.Tunnels)
			}
		})
	}()
}

// redisTerminal is the terminal of p, whose connection is client: it takes
// the screen of the terminal before it, and shows the server's command
// line.
func (a *App) redisTerminal(s *Session, p *pane, client *redis.Client, topts terminal.Options, db int) (*terminal.Terminal, *redisCLI, error) {
	cli := newRedisCLI(s.ctx, client, client.Addr(), db)
	held := make(chan struct{})
	topts.Conn = &heldConn{ReadWriteCloser: cli, ready: held}
	// The banner of the status says that the connection ended: the
	// terminal's own line would sit above the history a terminal
	// connected again keeps.
	topts.NoExitMessage = true
	topts.OnExit = func(int) { a.post(func() { a.paneExited(s, p) }) }
	term, err := terminal.New(topts)
	if err != nil {
		close(held)
		cli.Close()
		return nil, nil, err
	}
	p.restore(term)
	close(held)
	return term, cli, nil
}
