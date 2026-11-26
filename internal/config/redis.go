package config

import (
	"net"
	"strings"
)

// The protocols a host may speak. SSH is the default, for every host saved
// before the key existed.
const (
	ProtocolSSH   = "ssh"
	ProtocolRedis = "redis"
)

// RedisSettings are the fields of a Redis host, besides its address, port
// and password: whether it is a cluster, its seeds, and its database.
type RedisSettings struct {
	// Cluster is whether the host is a Redis Cluster, reached through its
	// Seeds rather than its address and port.
	Cluster bool `json:"cluster,omitempty"`
	// Seeds are the cluster's nodes, each "host:port", which the CLI asks
	// for the slot's node as it meets one.
	Seeds []string `json:"seeds,omitempty"`
	// DB is the standalone server's database, 0 for the first.
	DB int `json:"db,omitempty"`
}

// ProtocolOrDefault is the protocol the host speaks: its own, or SSH for
// the hosts saved before there was a choice, and for nil.
func (h *Host) ProtocolOrDefault() string {
	if h == nil || h.Protocol == "" {
		return ProtocolSSH
	}
	return h.Protocol
}

// IsRedis tells whether the host is a Redis server; false for nil.
func (h *Host) IsRedis() bool { return h != nil && h.Protocol == ProtocolRedis }

// RedisSeeds are the cluster's seed addresses, "host:port" each, without
// the empty lines and the spaces around them.
func (h *Host) RedisSeeds() []string {
	if h == nil || h.Redis == nil {
		return nil
	}
	out := make([]string, 0, len(h.Redis.Seeds))
	for _, s := range h.Redis.Seeds {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// AddrPort is the host's address and port as "host:port", port defaulting
// to def.
func (h *Host) AddrPort(def int) string {
	if h == nil {
		return ""
	}
	port := h.Port
	if port == 0 {
		port = def
	}
	return net.JoinHostPort(strings.TrimSpace(h.Addr), itoa(port))
}

// itoa is strconv.Itoa, kept here so that callers of AddrPort need no
// import of it.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
