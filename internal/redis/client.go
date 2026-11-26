// Package redis speaks the Redis protocol (RESP2): it sends commands to a
// standalone server or to a cluster, where a node that holds another slot
// redirects them, and returns their replies as they are.
package redis

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Options are how to reach a server.
type Options struct {
	// Addr is a standalone server's "host:port".
	Addr string
	// Password is sent with AUTH when not "".
	Password string
	// DB is selected on a standalone server, 0 for the first.
	DB int
	// Cluster is whether the host is a Redis Cluster, reached through
	// Seeds: Addr and DB are then ignored.
	Cluster bool
	// Seeds are the cluster's nodes, "host:port" each.
	Seeds []string
	// Timeout bounds connecting and each command; zero for 10 and 30
	// seconds.
	Timeout time.Duration
	// Dial opens a connection to an address when the server is reached
	// through a proxy or a jump host, which take each connection — and
	// each node of a cluster — of their own; nil dials it directly.
	Dial func(ctx context.Context, addr string) (net.Conn, error)
}

// Kind is what a reply is.
type Kind int

const (
	// Status is a simple string, as "OK".
	Status Kind = iota
	// Err is an error reply, its text without the leading "-".
	Err
	// Int is an integer.
	Int
	// Bulk is a string.
	Bulk
	// Array is a list, possibly empty.
	Array
	// Nil is the null bulk string or the null array.
	Nil
)

// Reply is one reply of the server.
type Reply struct {
	Kind  Kind
	Str   string
	Int   int64
	Items []Reply
}

// Error is the reply as an error, for a Kind of Err; nil for the others.
func (r Reply) Error() error {
	if r.Kind != Err {
		return nil
	}
	return errors.New(r.Str)
}

// Text is the reply as the CLI writes it for a status or an error.
func (r Reply) Text() string { return r.Str }

// Client is a connection to a server, or to the node of a cluster that
// answered last: a reply that says the key lives elsewhere moves it there.
type Client struct {
	mu     sync.Mutex
	opts   Options
	conn   net.Conn
	r      *bufio.Reader
	addr   string
	closed bool
}

const (
	defaultDialTimeout = 10 * time.Second
	defaultCmdTimeout  = 30 * time.Second
)

// Dial connects to the server, or to the first seed of the cluster that
// answers, logging in and selecting the database there.
func Dial(ctx context.Context, o Options) (*Client, error) {
	c := &Client{opts: o}
	var lastErr error
	for _, addr := range c.addresses() {
		if err := c.connect(ctx, addr); err != nil {
			lastErr = err
			continue
		}
		return c, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no address to connect to")
	}
	return nil, lastErr
}

// addresses are where Dial may connect: the address of a standalone server,
// else the cluster's seeds.
func (c *Client) addresses() []string {
	if !c.opts.Cluster {
		return []string{c.opts.Addr}
	}
	out := make([]string, 0, len(c.opts.Seeds))
	for _, s := range c.opts.Seeds {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// connect dials addr, replacing the connection there was, and logs in and
// selects the database.
func (c *Client) connect(ctx context.Context, addr string) error {
	timeout := c.opts.Timeout
	if timeout <= 0 {
		timeout = defaultDialTimeout
	}
	var conn net.Conn
	var err error
	if c.opts.Dial != nil {
		conn, err = c.opts.Dial(ctx, addr)
	} else {
		d := net.Dialer{Timeout: timeout}
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn, c.r, c.addr = conn, bufio.NewReader(conn), addr
	if c.opts.Password != "" {
		if _, err := c.do(ctx, []string{"AUTH", c.opts.Password}); err != nil {
			conn.Close()
			c.conn, c.r = nil, nil
			return fmt.Errorf("auth: %w", err)
		}
	}
	if !c.opts.Cluster && c.opts.DB != 0 {
		if _, err := c.do(ctx, []string{"SELECT", strconv.Itoa(c.opts.DB)}); err != nil {
			conn.Close()
			c.conn, c.r = nil, nil
			return fmt.Errorf("select %d: %w", c.opts.DB, err)
		}
	}
	return nil
}

// Addr is the node the client is connected to.
func (c *Client) Addr() string { return c.addr }

// Close closes the connection; later commands fail.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.conn != nil {
		err := c.conn.Close()
		c.conn, c.r = nil, nil
		return err
	}
	return nil
}

// Do sends one command and returns its reply, following a cluster's
// redirections once.
func (c *Client) Do(ctx context.Context, args ...string) (Reply, error) {
	if len(args) == 0 {
		return Reply{}, errors.New("empty command")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Reply{}, errors.New("closed")
	}
	reply, err := c.do(ctx, args)
	if err != nil {
		return Reply{}, err
	}
	if reply.Kind != Err || !c.opts.Cluster {
		return reply, nil
	}
	kind, addr := redirection(reply.Str)
	if kind == "" {
		return reply, nil
	}
	if kind == "ASK" {
		// The slot is moving: the node there answers once it is asked
		// to serve it. The same node stays.
		if _, err := c.do(ctx, []string{"ASKING"}); err != nil {
			return Reply{}, err
		}
		return c.do(ctx, args)
	}
	if addr == c.addr {
		return reply, nil
	}
	if err := c.connect(ctx, addr); err != nil {
		return Reply{}, err
	}
	return c.do(ctx, args)
}

// redirection reads the kind ("MOVED" or "ASK") and the node of a cluster's
// redirection, "" when the reply is not one.
func redirection(s string) (kind, addr string) {
	for _, k := range []string{"MOVED", "ASK"} {
		if strings.HasPrefix(s, k+" ") {
			f := strings.Fields(s)
			if len(f) == 3 {
				return k, f[2]
			}
		}
	}
	return "", ""
}

// do writes one command and reads its reply, on the connection there is.
func (c *Client) do(ctx context.Context, args []string) (Reply, error) {
	if c.conn == nil {
		return Reply{}, errors.New("not connected")
	}
	timeout := c.opts.Timeout
	if timeout <= 0 {
		timeout = defaultCmdTimeout
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	// A channel of an SSH connection, as a jump host opens, takes no
	// deadline: the command waits, and the context ends it by closing the
	// connection.
	timed := c.conn.SetDeadline(deadline) == nil
	var b []byte
	b = append(b, '*')
	b = strconv.AppendInt(b, int64(len(args)), 10)
	b = append(b, '\r', '\n')
	for _, a := range args {
		b = append(b, '$')
		b = strconv.AppendInt(b, int64(len(a)), 10)
		b = append(b, '\r', '\n')
		b = append(b, a...)
		b = append(b, '\r', '\n')
	}
	if _, err := c.conn.Write(b); err != nil {
		return Reply{}, err
	}
	if timed {
		return readReply(c.r)
	}
	type answer struct {
		reply Reply
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		r, err := readReply(c.r)
		done <- answer{r, err}
	}()
	select {
	case a := <-done:
		return a.reply, a.err
	case <-ctx.Done():
		c.conn.Close() // ends the read
		<-done
		return Reply{}, ctx.Err()
	}
}

// readReply reads one reply, nested arrays included.
func readReply(r *bufio.Reader) (Reply, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return Reply{}, err
	}
	if len(line) < 3 {
		return Reply{}, fmt.Errorf("short reply %q", line)
	}
	body := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch body[0] {
	case '+':
		return Reply{Kind: Status, Str: body[1:]}, nil
	case '-':
		return Reply{Kind: Err, Str: body[1:]}, nil
	case ':':
		n, err := strconv.ParseInt(body[1:], 10, 64)
		if err != nil {
			return Reply{}, fmt.Errorf("integer %q", body)
		}
		return Reply{Kind: Int, Int: n}, nil
	case '$':
		n, err := strconv.Atoi(body[1:])
		if err != nil {
			return Reply{}, fmt.Errorf("bulk length %q", body)
		}
		if n < 0 {
			return Reply{Kind: Nil}, nil
		}
		if n > 512<<20 {
			return Reply{}, fmt.Errorf("bulk length %d", n)
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return Reply{}, err
		}
		return Reply{Kind: Bulk, Str: string(buf[:n])}, nil
	case '*':
		n, err := strconv.Atoi(body[1:])
		if err != nil {
			return Reply{}, fmt.Errorf("array length %q", body)
		}
		if n < 0 {
			return Reply{Kind: Nil}, nil
		}
		if n > 1<<20 {
			return Reply{}, fmt.Errorf("array length %d", n)
		}
		items := make([]Reply, 0, min(n, 64))
		for i := 0; i < n; i++ {
			item, err := readReply(r)
			if err != nil {
				return Reply{}, err
			}
			items = append(items, item)
		}
		return Reply{Kind: Array, Items: items}, nil
	}
	return Reply{}, fmt.Errorf("unknown reply %q", body)
}
