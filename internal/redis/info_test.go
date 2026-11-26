package redis

import (
	"context"
	"strings"
	"testing"
)

// INFO's text is read into sections and their fields, in the server's
// order, with the colons of a value kept.
func TestParseInfo(t *testing.T) {
	text := strings.Join([]string{
		"# Server",
		"redis_version:7.2.4",
		"executable:/usr/bin/redis-server",
		"",
		"# Clients",
		"connected_clients:3",
		"# Keyspace",
		"db0:keys=12,expires=3,avg_ttl=0",
		"db2:keys=8,expires=0,avg_ttl=0",
		"# A Line That Is Not A Field",
		"",
	}, "\r\n")
	in := ParseInfo(text)
	if len(in.Sections) != 4 {
		t.Fatalf("sections %+v", in.Sections)
	}
	if in.Sections[0].Name != "Server" || len(in.Sections[0].Fields) != 2 {
		t.Errorf("server %+v", in.Sections[0])
	}
	// A section of a newer server keeps its text as its name, with no
	// fields, rather than losing what follows.
	if last := in.Sections[3]; last.Name != "A Line That Is Not A Field" || len(last.Fields) != 0 {
		t.Errorf("last %+v", last)
	}
	if got := in.Value("Server", "executable"); got != "/usr/bin/redis-server" {
		t.Errorf("executable %q", got)
	}
	if got := in.Int("Clients", "connected_clients"); got != 3 {
		t.Errorf("clients %d", got)
	}
	if got := in.Keys(); got != 20 {
		t.Errorf("keys %d, want 20", got)
	}
	// The databases of Keyspace, with the keys and the expiring of each.
	dbs := in.Databases()
	if len(dbs) != 2 {
		t.Fatalf("databases %+v", dbs)
	}
	if dbs[0] != (Database{Name: "db0", Keys: 12, Expires: 3}) || dbs[1] != (Database{Name: "db2", Keys: 8}) {
		t.Errorf("databases %+v", dbs)
	}
	if in.Section("Stats") != nil || in.Value("Stats", "x") != "" || in.Int("Stats", "x") != 0 {
		t.Error("a section the server did not send")
	}
	if got := ParseInfo("redis_version:1").Sections; len(got) != 0 {
		t.Errorf("a field before any section: %+v", got)
	}
}

// Info is the INFO command's text, read.
func TestClientInfo(t *testing.T) {
	addr := serve(t, func(args []string) Reply {
		if strings.ToUpper(args[0]) != "INFO" {
			return Reply{Kind: Err, Str: "ERR unknown command"}
		}
		return Reply{Kind: Bulk, Str: infoText}
	})
	c, err := Dial(context.Background(), Options{Addr: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	in, err := c.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := in.Value("Server", "redis_version"); got != "7.2.4" {
		t.Errorf("version %q; sections %+v", got, in.Sections)
	}
}

const infoText = "# Server\r\nredis_version:7.2.4\r\n"
