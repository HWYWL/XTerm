package adb

// Running a command on the device and hearing how it went: a file the
// device would not remove, or a folder it would not make, is an error, not
// the empty answer a command that worked gives. The shell service of older
// devices says nothing of a command's exit status, so it is asked for after
// the command; shell v2, which has a packet for it, is used where the
// device has it.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	goadb "github.com/pgaskin/go-adb/adb"
	"github.com/pgaskin/go-adb/adb/adbproto"
	"github.com/pgaskin/go-adb/adb/adbproto/shellproto2"
)

// CommandError is a command of the device that ended with a status other
// than 0: a file it could not remove, a folder it could not make.
type CommandError struct {
	Command string
	Status  int
	Output  string
}

func (e *CommandError) Error() string {
	if out := strings.TrimSpace(e.Output); out != "" {
		return out
	}
	return fmt.Sprintf("%s: exit status %d", e.Command, e.Status)
}

// statusMarker separates a command's output from the status asked for
// after it, on a device without shell v2.
const statusMarker = "__xterm_status:"

// Run is Exec for a command whose failure matters: it returns the error the
// command ended with, with what it printed, rather than the output alone.
func (c *Conn) Run(ctx context.Context, command string) (string, error) {
	out, status, err := c.run(ctx, command)
	if err != nil {
		return out, err
	}
	if status > 0 {
		return out, &CommandError{Command: command, Status: status, Output: out}
	}
	return out, nil
}

// run is the command's output and its status, -1 when the device did not
// say.
func (c *Conn) run(ctx context.Context, command string) (string, int, error) {
	if c == nil || c.dial == nil {
		return "", -1, errors.New("adb: no connection to run a command on")
	}
	if goadb.SupportsFeature(c.dial, adbproto.FeatureShell2) == nil {
		conn, err := c.shell2(ctx, command)
		if err == nil {
			return c.readShell2(conn)
		}
	}
	return c.run1(ctx, command)
}

// shell2 opens shell v2 over the connection, where the device has it.
func (c *Conn) shell2(ctx context.Context, command string) (*goadb.ShellConn2, error) {
	var b shellproto2.ServiceBuilder
	b.Raw()
	b.Command(command)
	conn, err := goadb.Shell2(ctx, c.dial, b.String())
	if err != nil {
		c.count.failed(ChannelExec)
		return nil, err
	}
	c.count.open(ChannelExec)
	return conn, nil
}

// readShell2 reads a shell v2 channel to its exit packet.
func (c *Conn) readShell2(conn *goadb.ShellConn2) (string, int, error) {
	defer func() {
		conn.Close()
		c.count.closed(ChannelExec)
	}()
	var b strings.Builder
	for {
		id, pkt, ok := conn.Read()
		if !ok {
			return b.String(), -1, conn.Error()
		}
		switch id {
		case shellproto2.PacketExit:
			status := -1
			if len(pkt) > 0 {
				status = int(pkt[0])
			}
			return b.String(), status, nil
		case shellproto2.PacketStdout, shellproto2.PacketStderr:
			if b.Len() < 256<<10 {
				b.Write(pkt)
			}
		}
	}
}

// run1 runs the command over the legacy shell, which does not report a
// status: it is echoed after the command, after its output.
func (c *Conn) run1(ctx context.Context, command string) (string, int, error) {
	nc, err := goadb.Shell(ctx, c.dial, command+`; echo "`+statusMarker+`$?"`)
	if err != nil {
		c.count.failed(ChannelExec)
		return "", -1, err
	}
	c.count.open(ChannelExec)
	defer func() {
		nc.Close()
		c.count.closed(ChannelExec)
	}()
	var b strings.Builder
	buf := make([]byte, 8<<10)
	for {
		n, rerr := nc.Read(buf)
		if n > 0 && b.Len() < 256<<10 {
			b.Write(buf[:n])
		}
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) {
				return b.String(), -1, rerr
			}
			break
		}
	}
	out := b.String()
	i := strings.LastIndex(out, statusMarker)
	if i < 0 {
		return out, -1, nil
	}
	status, perr := strconv.Atoi(strings.TrimSpace(out[i+len(statusMarker):]))
	if perr != nil {
		return out, -1, nil
	}
	return out[:i], status, nil
}
