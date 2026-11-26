package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A Redis host's fields are read and written, and the hosts saved before
// the protocol existed are SSH.
func TestRedisHostJSON(t *testing.T) {
	in := `{"id": "h1", "name": "cache", "addr": "10.0.0.1", "protocol": "redis", "redis": {"cluster": true, "seeds": ["10.0.0.1:6379", "10.0.0.2:6379"], "db": 2}}`
	var h Host
	if err := json.Unmarshal([]byte(in), &h); err != nil {
		t.Fatal(err)
	}
	if !h.IsRedis() || h.ProtocolOrDefault() != ProtocolRedis {
		t.Errorf("protocol %q, redis %v", h.Protocol, h.IsRedis())
	}
	want := &RedisSettings{Cluster: true, Seeds: []string{"10.0.0.1:6379", "10.0.0.2:6379"}, DB: 2}
	if !reflect.DeepEqual(h.Redis, want) {
		t.Errorf("redis %+v, want %+v", h.Redis, want)
	}
	data, _ := json.Marshal(h)
	var back struct {
		Protocol string         `json:"protocol"`
		Redis    map[string]any `json:"redis"`
	}
	json.Unmarshal(data, &back)
	if back.Protocol != "redis" || back.Redis["cluster"] != true || back.Redis["db"] != 2.0 {
		t.Errorf("written %s", data)
	}

	old := `{"id": "h2", "addr": "example.com"}`
	var o Host
	if err := json.Unmarshal([]byte(old), &o); err != nil {
		t.Fatal(err)
	}
	if o.ProtocolOrDefault() != ProtocolSSH || o.IsRedis() {
		t.Errorf("an old host speaks %q", o.ProtocolOrDefault())
	}
	var nilHost *Host
	if nilHost.ProtocolOrDefault() != ProtocolSSH || nilHost.IsRedis() {
		t.Error("a nil host is not SSH")
	}
}

// The seeds are "host:port" each, without the empty lines, and Target says
// where a Redis host is.
func TestRedisSeedsAndTarget(t *testing.T) {
	h := &Host{Addr: "10.0.0.1", Protocol: ProtocolRedis, Redis: &RedisSettings{Seeds: []string{" 10.0.0.1:6379 ", "", "10.0.0.2:6379"}}}
	want := []string{"10.0.0.1:6379", "10.0.0.2:6379"}
	if got := h.RedisSeeds(); !reflect.DeepEqual(got, want) {
		t.Errorf("seeds %q, want %q", got, want)
	}
	if got := h.AddrPort(6379); got != "10.0.0.1:6379" {
		t.Errorf("address %q", got)
	}
	if got := h.Target(); got != "10.0.0.1:6379" {
		t.Errorf("target %q", got)
	}
	h.Redis.Cluster = true
	if got := h.Target(); got != "10.0.0.1:6379 +1" {
		t.Errorf("cluster target %q", got)
	}
	h.Redis.Seeds = h.Redis.Seeds[:1]
	if got := h.Target(); got != "10.0.0.1:6379" {
		t.Errorf("one seed: %q", got)
	}
	if got := (&Host{Addr: "10.0.0.9", Protocol: ProtocolRedis}).Target(); got != "10.0.0.9:6379" {
		t.Errorf("standalone target %q", got)
	}
	if got := (&Host{Addr: "x", User: "root", Protocol: ProtocolRedis}).Target(); got != "x:6379" {
		t.Errorf("no user in a Redis target: %q", got)
	}
}
