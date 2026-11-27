package app

import "testing"

// ss's rows become sockets, with the process that holds them.
func TestParseADBSocketsSS(t *testing.T) {
	out := `Netid State  Recv-Q Send-Q Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      4096   127.0.0.1:5037      0.0.0.0:*         users:(("adbd",pid=123,fd=10))
tcp   ESTAB  0      0      192.168.1.5:43210   93.184.216.34:443 users:(("com.foo",pid=456,fd=55))
udp   UNCONN 0      0      0.0.0.0:68          0.0.0.0:*
`
	list, err := parseADBSockets(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("%d sockets: %+v", len(list), list)
	}
	if s := list[0]; s.Proto != "tcp" || s.State != "LISTEN" || s.Local != "127.0.0.1:5037" ||
		s.Remote != "0.0.0.0:*" || s.PID != 123 || s.Program != "adbd" {
		t.Errorf("the listener is %+v", s)
	}
	if s := list[1]; s.PID != 456 || s.Program != "com.foo" {
		t.Errorf("the connection is %+v", s)
	}
}

// netstat's rows do, with or without its PID/Program column.
func TestParseADBSocketsNetstat(t *testing.T) {
	out := `Proto Recv-Q Send-Q Local Address Foreign Address State PID/Program name
tcp 0 0 0.0.0.0:5555 0.0.0.0:* LISTEN 1234/adbd
tcp 0 0 10.0.0.2:443 10.0.0.9:51234 ESTABLISHED -
udp 0 0 0.0.0.0:68 0.0.0.0:* -
`
	list, err := parseADBSockets(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("%d sockets: %+v", len(list), list)
	}
	if s := list[0]; s.Local != "0.0.0.0:5555" || s.State != "LISTEN" || s.PID != 1234 || s.Program != "adbd" {
		t.Errorf("the listener is %+v", s)
	}
	if s := list[2]; s.Proto != "udp" || s.PID != 0 {
		t.Errorf("the udp socket is %+v", s)
	}
}

// A device with neither tool says so.
func TestParseADBSocketsNoTools(t *testing.T) {
	if _, err := parseADBSockets("__XTERM_NO_NET__\n"); err == nil {
		t.Error("no error without ss or netstat")
	}
}
