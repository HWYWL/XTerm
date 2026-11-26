package redis

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// serve runs a server on a loopback port, answering each command with what
// f returns, and returns its address.
func serve(t *testing.T, f func(args []string) Reply) string {
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
				w := bufio.NewWriter(c)
				for {
					args, err := readArgs(r)
					if err != nil {
						return
					}
					writeReply(w, f(args))
					w.Flush()
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

// readArgs reads one command, as the server does.
func readArgs(r *bufio.Reader) ([]string, error) {
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
		if _, err := r.ReadString('\n'); err != nil { // $len
			return nil, err
		}
		buf := make([]byte, 0, 16)
		for {
			c, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if c == '\r' {
				if _, err := r.ReadByte(); err != nil { // \n
					return nil, err
				}
				break
			}
			buf = append(buf, c)
		}
		args = append(args, string(buf))
	}
	return args, nil
}

// writeReply writes one reply, as the server does.
func writeReply(w *bufio.Writer, r Reply) {
	switch r.Kind {
	case Status:
		fmt.Fprintf(w, "+%s\r\n", r.Str)
	case Err:
		fmt.Fprintf(w, "-%s\r\n", r.Str)
	case Int:
		fmt.Fprintf(w, ":%d\r\n", r.Int)
	case Bulk:
		fmt.Fprintf(w, "$%d\r\n%s\r\n", len(r.Str), r.Str)
	case Nil:
		w.WriteString("$-1\r\n")
	case Array:
		fmt.Fprintf(w, "*%d\r\n", len(r.Items))
		for _, it := range r.Items {
			writeReply(w, it)
		}
	}
}

// The replies come back as the server sent them.
func TestDoReadsReplies(t *testing.T) {
	addr := serve(t, func(args []string) Reply {
		switch args[0] {
		case "S":
			return Reply{Kind: Status, Str: "OK"}
		case "I":
			return Reply{Kind: Int, Int: 42}
		case "B":
			return Reply{Kind: Bulk, Str: "hi"}
		case "NIL":
			return Reply{Kind: Nil}
		case "E":
			return Reply{Kind: Err, Str: "ERR bad"}
		case "A":
			return Reply{Kind: Array, Items: []Reply{
				{Kind: Int, Int: 1}, {Kind: Bulk, Str: "x"},
			}}
		}
		return Reply{Kind: Err, Str: "ERR unknown"}
	})
	c, err := Dial(context.Background(), Options{Addr: addr, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c.Close()
	ctx := context.Background()
	for _, tc := range []struct {
		cmd  string
		kind Kind
		text string
	}{
		{"S", Status, "OK"},
		{"I", Int, ""},
		{"B", Bulk, "hi"},
		{"NIL", Nil, ""},
		{"E", Err, "ERR bad"},
		{"A", Array, ""},
	} {
		r, err := c.Do(ctx, tc.cmd)
		if err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		if r.Kind != tc.kind {
			t.Errorf("%s: kind %v, want %v", tc.cmd, r.Kind, tc.kind)
		}
		if tc.text != "" && r.Str != tc.text {
			t.Errorf("%s: %q, want %q", tc.cmd, r.Str, tc.text)
		}
	}
}

// Dial logs in and selects the database before any command.
func TestDialAuthAndSelect(t *testing.T) {
	var mu sync.Mutex
	var got []string
	addr := serve(t, func(args []string) Reply {
		mu.Lock()
		got = append(got, strings.Join(args, " "))
		mu.Unlock()
		return Reply{Kind: Status, Str: "OK"}
	})
	c, err := Dial(context.Background(), Options{Addr: addr, Password: "pw", DB: 3, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c.Close()
	if _, err := c.Do(context.Background(), "PING"); err != nil {
		t.Fatalf("%v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"AUTH pw", "SELECT 3"}
	if len(got) < 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the server got %q, want %q first", got, want)
	}
}

// A cluster's redirection moves the command to the node it names.
func TestClusterFollowsMoved(t *testing.T) {
	b := serve(t, func(args []string) Reply { return Reply{Kind: Bulk, Str: "v"} })
	a := serve(t, func(args []string) Reply { return Reply{Kind: Err, Str: "MOVED 0 " + b} })
	c, err := Dial(context.Background(), Options{Cluster: true, Seeds: []string{a}, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c.Close()
	r, err := c.Do(context.Background(), "GET", "k")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if r.Kind != Bulk || r.Str != "v" {
		t.Errorf("reply %+v", r)
	}
	if c.Addr() != b {
		t.Errorf("connected to %q, want %q", c.Addr(), b)
	}
}

// A cluster's seeds are tried until one answers.
func TestClusterTriesSeeds(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("%v", err)
	}
	dead := ln.Addr().String()
	ln.Close() // nothing listens there
	ok := serve(t, func(args []string) Reply { return Reply{Kind: Status, Str: "PONG"} })
	c, err := Dial(context.Background(), Options{Cluster: true, Seeds: []string{dead, ok}, Timeout: time.Second})
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c.Close()
	if c.Addr() != ok {
		t.Errorf("connected to %q, want %q", c.Addr(), ok)
	}
}
