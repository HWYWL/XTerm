package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/redis"
	"finallyshell/locales"

	"github.com/egoist/mygo/plugins/terminal"
)

// redisServer runs a Redis server on a loopback port, answering each
// command with the reply reply returns, and returns its address.
func redisServer(t *testing.T, reply func(args []string) string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					args, err := readRedisCommand(r)
					if err != nil {
						return
					}
					if _, err := io.WriteString(c, reply(args)); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

// readRedisCommand reads one command of a client, as a server does.
func readRedisCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if len(line) == 0 || line[0] != '*' {
		return nil, fmt.Errorf("not a command: %q", line)
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if len(line) == 0 || line[0] != '$' {
			return nil, fmt.Errorf("not a bulk: %q", line)
		}
		size, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:size]))
	}
	return args, nil
}

// socksServer runs a SOCKS5 server without authentication on a loopback
// port, opening what it is asked through it, and returns its address.
func socksServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				hello := make([]byte, 2)
				if _, err := io.ReadFull(r, hello); err != nil || hello[0] != 5 {
					return
				}
				if _, err := io.CopyN(io.Discard, r, int64(hello[1])); err != nil {
					return
				}
				if _, err := c.Write([]byte{5, 0}); err != nil { // no authentication
					return
				}
				req := make([]byte, 4)
				if _, err := io.ReadFull(r, req); err != nil || req[1] != 1 {
					return
				}
				var host string
				switch req[3] {
				case 1:
					ip := make([]byte, 4)
					if _, err := io.ReadFull(r, ip); err != nil {
						return
					}
					host = net.IP(ip).String()
				case 3:
					n, err := r.ReadByte()
					if err != nil {
						return
					}
					name := make([]byte, n)
					if _, err := io.ReadFull(r, name); err != nil {
						return
					}
					host = string(name)
				default:
					return
				}
				b := make([]byte, 2)
				if _, err := io.ReadFull(r, b); err != nil {
					return
				}
				port := int(b[0])<<8 | int(b[1])
				out, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
				if err != nil {
					c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer out.Close()
				if _, err := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
					return
				}
				go io.Copy(out, r)
				io.Copy(c, out)
			}(c)
		}
	}()
	return ln.Addr().String()
}

// redisReply is a server of the tests: PING, GET and SET, as a cache needs
// them.
func redisReply(args []string) string {
	switch strings.ToUpper(args[0]) {
	case "PING":
		return "+PONG\r\n"
	case "GET":
		if len(args) == 2 && args[1] == "k" {
			return "$5\r\nvalue\r\n"
		}
		return "$-1\r\n"
	case "SET":
		return "+OK\r\n"
	case "AUTH":
		return "+OK\r\n"
	case "SELECT":
		return "+OK\r\n"
	}
	return "-ERR unknown command\r\n"
}

// redisHost is a host of the tests at addr, a standalone server.
func redisHost(t *testing.T, addr string) *config.Host {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	n, _ := strconv.Atoi(port)
	return &config.Host{
		ID: config.NewHostID(), Name: "cache", Addr: host, Port: n,
		Protocol: config.ProtocolRedis, Auth: config.AuthPassword,
	}
}

// A Redis session shows the server's command line in the terminal, and has
// no bottom panel: the protocol has no files or monitor.
func TestRedisSession(t *testing.T) {
	if err := terminal.Load(); err != nil {
		t.Skip("no terminal:", err)
	}
	addr := redisServer(t, func(args []string) string {
		if strings.EqualFold(args[0], "GET") && len(args) == 2 && args[1] == "名字" {
			return "$6\r\n中文\r\n"
		}
		return redisReply(args)
	})
	a, p, tt, _ := startApp(t)
	h := redisHost(t, addr)
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	p.until(t, "the session to connect", func() bool {
		s := a.current()
		return s != nil && s.isRedis() && s.status == statusConnected
	})
	s := a.current()
	untilScreen(t, p, s, strings.Fields(locales.RedisConnected(addr))[0])
	cli := s.panes.panes()[0].redis
	if cli == nil {
		t.Fatal("the pane has no command line")
	}
	cli.Write([]byte("PING\r"))
	untilScreen(t, p, s, "PONG")
	cli.Write([]byte("GET k\r"))
	untilScreen(t, p, s, `"value"`)
	// Chinese is typed and shown as it is, and reaches the server whole.
	cli.Write([]byte("SET 名字 中文\r"))
	untilScreen(t, p, s, "OK")
	cli.Write([]byte("GET 名字\r"))
	untilScreen(t, p, s, `"中文"`)

	tt.Frame()
	saveShot(t, tt, "redis-session-en.png")
	if tt.HasText(locales.OpenPanelTab()) {
		t.Errorf("a Redis tab has the panel's button; texts %q", tt.Texts())
	}
	a.openBottomTab(tabFiles)
	if len(a.notices) == 0 || a.notices[len(a.notices)-1] != locales.RedisNoPanel() {
		t.Errorf("no notice for the panel: %q", a.notices)
	}
}

// A Redis host that cannot be reached directly connects through its SOCKS5
// proxy: the connection, and a cluster's each, goes through it.
func TestRedisSessionThroughProxy(t *testing.T) {
	if err := terminal.Load(); err != nil {
		t.Skip("no terminal:", err)
	}
	addr := redisServer(t, redisReply)
	proxy := socksServer(t)
	pHost, pPort, err := net.SplitHostPort(proxy)
	if err != nil {
		t.Fatalf("%v", err)
	}
	pn, _ := strconv.Atoi(pPort)
	rHost, rPort, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	rn, _ := strconv.Atoi(rPort)
	a, p, _, _ := startApp(t)
	h := &config.Host{
		ID: config.NewHostID(), Name: "cache", Addr: rHost, Port: rn,
		Protocol: config.ProtocolRedis, Auth: config.AuthPassword,
		Proxy: &config.ProxyConfig{Kind: config.ProxySOCKS5, Addr: pHost, Port: pn},
	}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	p.until(t, "the session to connect", func() bool {
		s := a.current()
		return s != nil && s.isRedis() && s.status == statusConnected
	})
	s := a.current()
	untilScreen(t, p, s, strings.Fields(locales.RedisConnected(addr))[0])
	cli := s.panes.panes()[0].redis
	if cli == nil {
		t.Fatal("the pane has no command line")
	}
	cli.Write([]byte("PING\r"))
	untilScreen(t, p, s, "PONG")
}

// A Redis host behind a jump host reaches its server through the jump
// host's connection, which also carries its tunnels.
func TestRedisSessionThroughJumpHost(t *testing.T) {
	if err := terminal.Load(); err != nil {
		t.Skip("no terminal:", err)
	}
	addr := redisServer(t, redisReply)
	jump := startJumpSSH(t)
	a, p, _, _ := startApp(t)
	jh := &config.Host{ID: config.NewHostID(), Name: "bastion", Addr: jump.host, Port: jump.port, User: "jump", Auth: config.AuthPassword}
	a.hosts.Add(jh)
	a.passwords[jh.ID] = "secret"
	rHost, rPort, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("%v", err)
	}
	rn, _ := strconv.Atoi(rPort)
	h := &config.Host{
		ID: config.NewHostID(), Name: "cache", Addr: rHost, Port: rn,
		Protocol: config.ProtocolRedis, Auth: config.AuthPassword,
		Proxy: jumpTo(jh.ID),
	}
	a.hosts.Add(h)
	a.openSessionIn(nil, h)
	p.until(t, "the session", func() bool { return len(a.sessions) == 2 })
	s := a.sessions[1]
	answerHostKeys(t, p, s)
	s = a.current()
	if s == nil || !s.isRedis() || s.status != statusConnected {
		t.Fatalf("status %v, err %v", s.status, s.err)
	}
	untilScreen(t, p, s, strings.Fields(locales.RedisConnected(addr))[0])
	cli := s.panes.panes()[0].redis
	if cli == nil {
		t.Fatal("the pane has no command line")
	}
	cli.Write([]byte("PING\r"))
	untilScreen(t, p, s, "PONG")
	if s.client == nil {
		t.Error("no jump host connection for the tunnels")
	}
}

// Choosing Redis in the editor saves a Redis host: its address, port,
// database and password.
func TestEditorSavesRedisHost(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.ProtocolRedis()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Frame()
	if !tt.HasText(locales.EditorPageRedis()) {
		t.Fatalf("no Redis page; texts %q", tt.Texts())
	}
	if tt.HasText(locales.FieldUser()) {
		t.Error("the Redis page asks for a user")
	}
	typeInto(t, tt, locales.FieldHost(), "127.0.0.1")
	typeInto(t, tt, locales.FieldPort(), "6380")
	typeInto(t, tt, locales.FieldRedisDB(), "2")
	typeInto(t, tt, locales.FieldPassword(), "pw")
	saveShot(t, tt, "editor-redis-en.png")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if a.windowOpen(WindowEditor) {
		t.Fatalf("the editor is open: %q", tt.Texts())
	}
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	h := a.hosts.Hosts[0]
	if h.ProtocolOrDefault() != config.ProtocolRedis || !h.IsRedis() || h.Addr != "127.0.0.1" || h.Port != 6380 {
		t.Errorf("host %+v", h)
	}
	if h.Redis == nil || h.Redis.Cluster || h.Redis.DB != 2 || h.User != "" {
		t.Errorf("redis %+v, user %q", h.Redis, h.User)
	}
	if a.passwords[h.ID] != "pw" {
		t.Errorf("password %q", a.passwords[h.ID])
	}
	if h.Target() != "127.0.0.1:6380" {
		t.Errorf("target %q", h.Target())
	}
}

// A Redis host has the pages of SSH besides its own: its command line is a
// terminal, it may be reached through a proxy or a jump host, and its
// tunnels run on the jump host's connection.
func TestEditorRedisPages(t *testing.T) {
	a, p := testApp(t)
	a.newConnection()
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.ProtocolRedis()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	for _, page := range []string{locales.EditorPageTerminal(), locales.EditorPageProxy(),
		locales.EditorPageTunnels(), locales.EditorPageJumpHost()} {
		if !tt.HasText(page) {
			t.Errorf("%q is not offered; texts %q", page, tt.Texts())
		}
	}
	saveShot(t, tt, "editor-redis-pages-en.png")
	// Tunnels need a jump host, and the page says so.
	if err := tt.Click(locales.EditorPageTunnels()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.RedisTunnelsNote()) {
		t.Errorf("no note without a jump host; texts %q", tt.Texts())
	}
	saveShot(t, tt, "editor-redis-tunnels-en.png")

	// A SOCKS5 proxy is saved with the host: the Redis connection goes
	// through it, as a cluster's nodes do too.
	if err := tt.Click("Redis"); err != nil {
		t.Fatal(err)
	}
	typeInto(t, tt, locales.FieldHost(), "10.0.0.1")
	typeInto(t, tt, locales.FieldPort(), "6379")
	if err := tt.Click(locales.EditorPageProxy()); err != nil {
		t.Fatal(err)
	}
	choose(t, tt, locales.FieldProxyType(), "SOCKS5")
	typeInto(t, tt, locales.FieldHost(), "198.51.100.7")
	typeInto(t, tt, locales.FieldPort(), "1081")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatalf("%v", err)
	}
	tt.Frame()
	if len(a.hosts.Hosts) != 1 {
		t.Fatalf("hosts %v", a.hosts.Hosts)
	}
	h := a.hosts.Hosts[0]
	if !h.IsRedis() {
		t.Errorf("protocol %q", h.Protocol)
	}
	if h.Proxy == nil || h.Proxy.Kind != config.ProxySOCKS5 || h.Proxy.Addr != "198.51.100.7" || h.Proxy.Port != 1081 {
		t.Errorf("proxy %+v", h.Proxy)
	}
}

// A cluster's seeds are what a cluster host is saved with, each host:port;
// one that is not is an error.
func TestRedisClusterSeeds(t *testing.T) {
	ed := &connEditor{protocol: editorRedis, cluster: true, seeds: "10.0.0.1:6379\n\n10.0.0.2:6379\n", db: "0"}
	if errs := ed.problems(); len(errs) != 0 {
		t.Fatalf("problems %v", errs)
	}
	a, _ := testApp(t)
	h := &config.Host{ID: config.NewHostID()}
	a.saveRedis(h, ed)
	if h.Redis == nil || !h.Redis.Cluster || len(h.Redis.Seeds) != 2 || h.Port != 0 {
		t.Fatalf("host %+v, redis %+v", h, h.Redis)
	}
	if h.Target() != "10.0.0.1:6379 +1" {
		t.Errorf("target %q", h.Target())
	}

	ed.seeds = "10.0.0.1\n"
	if errs := ed.problems(); errs[fieldSeeds] != locales.ErrRedisSeedInvalid("10.0.0.1") {
		t.Errorf("problems %v", errs)
	}
	ed.seeds = " \n"
	if errs := ed.problems(); errs[fieldSeeds] != locales.ErrRedisSeedsRequired() {
		t.Errorf("problems %v", errs)
	}
}

// A Redis command line reads a line as redis-cli does.
func TestSplitRedisCommand(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		{"GET k", []string{"GET", "k"}},
		{`SET k "a b"`, []string{"SET", "k", "a b"}},
		{`SET k 'a "b"'`, []string{"SET", "k", `a "b"`}},
		{`SET k a\ b`, []string{"SET", "k", "a b"}},
		{"  PING  ", []string{"PING"}},
		{"GET ", []string{"GET"}},
	} {
		got, err := splitRedisCommand(tc.line)
		if err != nil {
			t.Fatalf("%q: %v", tc.line, err)
		}
		if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Errorf("%q: %q, want %q", tc.line, got, tc.want)
		}
	}
	if _, err := splitRedisCommand(`GET "k`); err == nil {
		t.Error("an unterminated quote is taken")
	}
}

// A reply is written as redis-cli writes it: numbered arrays, quoted
// strings, errors in red.
func TestFormatRedisReply(t *testing.T) {
	got := formatRedisReply(redis.Reply{Kind: redis.Array, Items: []redis.Reply{
		{Kind: redis.Int, Int: 1},
		{Kind: redis.Bulk, Str: "a\nb"},
		{Kind: redis.Nil},
		{Kind: redis.Err, Str: "ERR bad"},
	}})
	for _, want := range []string{"1) (integer) 1", `2) "a\nb"`, "3) (nil)", "(error) ERR bad"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
	if got := formatRedisReply(redis.Reply{Kind: redis.Array}); !strings.Contains(got, "(empty array)") {
		t.Errorf("empty array %q", got)
	}
}

// The command line shows what the server answers, and says when it leaves.
func TestRedisCLI(t *testing.T) {
	addr := redisServer(t, redisReply)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := redis.Dial(ctx, redis.Options{Addr: addr})
	if err != nil {
		t.Fatalf("%v", err)
	}
	cli := newRedisCLI(ctx, client, addr, 0)
	defer cli.Close()
	shown := &shownText{}
	go shown.read(cli)
	shown.until(t, "the connection", "Connected to")
	cli.Write([]byte("PING\r"))
	shown.until(t, "PONG", "PONG")
	cli.Write([]byte("GET k\r"))
	shown.until(t, "the value", `"value"`)
	cli.Write([]byte("GET none\r"))
	shown.until(t, "nil", "(nil)")
	cli.Write([]byte("quit\r"))
	shown.until(t, "the end", locales.RedisBye())
	if !cli.quitFlag.Load() {
		t.Error("the command line did not say it left")
	}
}

// The line is edited where the cursor is: the arrows move it by whole
// characters, and Delete and Backspace take one.
func TestRedisCLIEditing(t *testing.T) {
	addr := redisServer(t, redisReply)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := redis.Dial(ctx, redis.Options{Addr: addr})
	if err != nil {
		t.Fatalf("%v", err)
	}
	cli := newRedisCLI(ctx, client, addr, 0)
	defer cli.Close()
	go (&shownText{}).read(cli) // what it shows is not read here

	cli.Write([]byte("abc"))
	if got := string(cli.line); got != "abc" || cli.pos != 3 {
		t.Fatalf("typed %q at %d", got, cli.pos)
	}
	cli.Write([]byte("\x1b[D\x1b[D")) // left twice
	if cli.pos != 1 {
		t.Fatalf("left twice: cursor %d", cli.pos)
	}
	cli.Write([]byte("X"))
	if got := string(cli.line); got != "aXbc" {
		t.Fatalf("typing in the middle: %q", got)
	}
	// The cursor is after X: Delete takes what follows it, the b; Left
	// then Delete takes the X.
	cli.Write([]byte("\x1b[3~"))
	if got := string(cli.line); got != "aXc" {
		t.Fatalf("delete after the cursor: %q", got)
	}
	cli.Write([]byte("\x1b[D\x1b[3~"))
	if got := string(cli.line); got != "ac" {
		t.Fatalf("delete at the cursor: %q", got)
	}
	cli.Write([]byte("\x1b[F\x7f")) // End, Backspace
	if got := string(cli.line); got != "a" {
		t.Fatalf("end and backspace: %q", got)
	}
	cli.Write([]byte("\x1b[H\x1b[3~")) // Home, Delete
	if got := string(cli.line); got != "" {
		t.Fatalf("home and delete: %q", got)
	}
	cli.Write([]byte("\x01")) // Ctrl+A, then Ctrl+K
	cli.Write([]byte("\x0b"))
	if got := string(cli.line); got != "" {
		t.Fatalf("after Ctrl+K: %q", got)
	}
	// The arrows of an application cursor mode, ESC O, and those of a
	// key with a modifier move the cursor too.
	cli.Write([]byte("abc"))
	cli.Write([]byte("\x1bOD\x1bOC")) // left, right
	if cli.pos != 3 {
		t.Errorf("ESC O arrows: cursor %d", cli.pos)
	}
	cli.Write([]byte("\x1b[1;5D")) // Ctrl+Left
	if cli.pos != 2 {
		t.Errorf("Ctrl+Left: cursor %d", cli.pos)
	}
}

// A character of several bytes is kept whole, and sent as it was typed:
// 中文 is 中文, not the bytes of it read as Latin-1.
func TestRedisCLIChinese(t *testing.T) {
	got := make(chan []string, 4)
	addr := redisServer(t, func(args []string) string {
		select {
		case got <- args:
		default:
		}
		return "+OK\r\n"
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := redis.Dial(ctx, redis.Options{Addr: addr})
	if err != nil {
		t.Fatalf("%v", err)
	}
	cli := newRedisCLI(ctx, client, addr, 0)
	defer cli.Close()
	go (&shownText{}).read(cli)

	// An input method may send the bytes of one character one at a time.
	for _, b := range []byte("中文") {
		cli.Write([]byte{b})
	}
	if got := string(cli.line); got != "中文" {
		t.Fatalf("typed %q, want 中文", got)
	}
	if cli.pos != len("中文") {
		t.Errorf("cursor %d, want %d", cli.pos, len("中文"))
	}
	// The arrows step over it, never into it.
	cli.Write([]byte("\x1b[D"))
	if cli.pos != len("中") {
		t.Errorf("left: cursor %d, want %d", cli.pos, len("中"))
	}
	cli.Write([]byte("\x7f"))
	if got := string(cli.line); got != "文" {
		t.Errorf("backspace: %q", got)
	}

	cli.Write([]byte("\x15")) // Ctrl+U clears what is left
	cli.Write([]byte("SET 名 中文\r"))
	select {
	case args := <-got:
		if strings.Join(args, " ") != "SET 名 中文" {
			t.Errorf("the server got %q", args)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the command did not reach the server")
	}
}

// A wide character takes two columns, as the terminal draws it.
func TestRedisCells(t *testing.T) {
	for _, tc := range []struct {
		s string
		n int
	}{
		{"abc", 3},
		{"中文", 4},
		{"a中b", 4},
		{"", 0},
	} {
		if got := cells([]byte(tc.s)); got != tc.n {
			t.Errorf("cells(%q) = %d, want %d", tc.s, got, tc.n)
		}
	}
}

// shownText is what a command line wrote, as a reader of it.
type shownText struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *shownText) read(r io.Reader) {
	b := make([]byte, 4096)
	for {
		n, err := r.Read(b)
		s.mu.Lock()
		s.buf.Write(b[:n])
		s.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (s *shownText) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *shownText) until(t *testing.T, what, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.text(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: %q is not shown; got %q", what, want, s.text())
}
