package adb

import (
	"net"
	"testing"

	goadb "github.com/pgaskin/go-adb/adb"
	"github.com/pgaskin/go-adb/adb/adbproto/shellproto2"
)

// A command that fails comes back with its status and what it printed, as
// rm does when the device does not let it remove the file.
func TestRunStatus(t *testing.T) {
	client, device := net.Pipe()
	t.Cleanup(func() { device.Close() })
	go func() {
		srv := shellproto2.New(device)
		srv.Write(shellproto2.PacketStdout, []byte("rm: "))
		srv.Write(shellproto2.PacketStderr, []byte("Permission denied"))
		srv.Write(shellproto2.PacketExit, []byte{1})
	}()
	c := &Conn{count: &counters{}}
	out, status, err := c.readShell2(goadb.NewShellConn2(client))
	if err != nil {
		t.Fatalf("the command: %v", err)
	}
	if status != 1 {
		t.Errorf("the status is %d", status)
	}
	if out != "rm: Permission denied" {
		t.Errorf("the output is %q", out)
	}
	if msg := (&CommandError{Command: "rm", Status: 1, Output: out}).Error(); msg != out {
		t.Errorf("the error reads %q", msg)
	}
}
