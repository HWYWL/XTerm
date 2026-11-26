package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Info is what a server says of itself: the sections of the INFO command,
// in the order the server prints them, which a side panel shows
// (internal/app/redispanel.go).
type Info struct {
	Sections []InfoSection
}

// InfoSection is one section of INFO — "# Memory", say — and its fields,
// which keep the server's order.
type InfoSection struct {
	Name   string
	Fields []InfoField
}

// InfoField is one line of a section: "used_memory:1234", where the value
// may hold colons, as "executable:/usr/bin/redis-server" does.
type InfoField struct {
	Key, Value string
}

// Info runs INFO and reads what the server says.
func (c *Client) Info(ctx context.Context) (Info, error) {
	r, err := c.Do(ctx, "INFO")
	if err != nil {
		return Info{}, err
	}
	if r.Kind != Bulk {
		return Info{}, fmt.Errorf("INFO: %s", r.Str)
	}
	return ParseInfo(r.Str), nil
}

// ParseInfo reads the text INFO prints: a section at "# Name", and the
// fields under it at "key:value". Lines that are neither are left out, as
// the server's newest sections may add some.
func ParseInfo(text string) Info {
	var in Info
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "# "):
			in.Sections = append(in.Sections, InfoSection{Name: strings.TrimSpace(line[2:])})
			continue
		}
		if len(in.Sections) == 0 {
			continue // a field before any section
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		last := &in.Sections[len(in.Sections)-1]
		last.Fields = append(last.Fields, InfoField{Key: key, Value: value})
	}
	return in
}

// Section returns the section of that name, nil for none.
func (in Info) Section(name string) *InfoSection {
	for i := range in.Sections {
		if in.Sections[i].Name == name {
			return &in.Sections[i]
		}
	}
	return nil
}

// Value returns what section says of key, "" when it says nothing.
func (in Info) Value(section, key string) string {
	s := in.Section(section)
	if s == nil {
		return ""
	}
	for _, f := range s.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// Int returns what section says of key as a number, 0 when it says nothing
// or not a number.
func (in Info) Int(section, key string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(in.Value(section, key)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// Keys is how many keys the databases hold, as the Keyspace section counts
// them.
func (in Info) Keys() int64 {
	var n int64
	for _, db := range in.Databases() {
		n += db.Keys
	}
	return n
}

// Database is one database as Keyspace counts it: "db0:keys=1,expires=0,
// avg_ttl=0".
type Database struct {
	Name    string
	Keys    int64
	Expires int64
}

// Databases returns the databases of the Keyspace section, in the server's
// order, which the panel draws a bar for each of.
func (in Info) Databases() []Database {
	s := in.Section("Keyspace")
	if s == nil {
		return nil
	}
	dbs := make([]Database, 0, len(s.Fields))
	for _, f := range s.Fields {
		db := Database{Name: f.Key}
		for _, part := range strings.Split(f.Value, ",") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				continue
			}
			switch key {
			case "keys":
				db.Keys = n
			case "expires":
				db.Expires = n
			}
		}
		dbs = append(dbs, db)
	}
	return dbs
}
