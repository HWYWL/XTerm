package config

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

// KnownHostsFile is the file under the config directory that sshx trusts, in
// OpenSSH's known_hosts format (hostkeys.go in internal/app reads it).
const KnownHostsFile = "known_hosts"

// KnownHostEntry is a line of that file: the hosts it trusts, the key, and
// the line it is on, for deleting it.
type KnownHostEntry struct {
	Marker  string   // "@cert-authority" or "@revoked", "" for a plain line
	Hosts   []string // the patterns of the line, or one hashed one
	KeyType string   // ssh-ed25519 and the like
	Key     string   // the base64 blob
	Line    int      // 1-based, as an editor counts lines
}

// Fingerprint is the key as ssh-keygen -l shows it: SHA256: of the blob.
func (e KnownHostEntry) Fingerprint() string {
	blob, err := base64.StdEncoding.DecodeString(e.Key)
	if err != nil {
		return e.Key // a line ssh itself would not take either
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// Hashed tells whether the hosts are hashed, as HashKnownHosts leaves them:
// the line then names no host to show.
func (e KnownHostEntry) Hashed() bool {
	return len(e.Hosts) > 0 && strings.HasPrefix(e.Hosts[0], "|")
}

// LoadKnownHosts reads the entries; a missing file is none.
func LoadKnownHosts() ([]KnownHostEntry, error) {
	data, err := os.ReadFile(filepath.Join(Dir(), KnownHostsFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []KnownHostEntry
	for i, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		e := KnownHostEntry{Line: i + 1}
		if strings.HasPrefix(fields[0], "@") {
			e.Marker, fields = fields[0], fields[1:]
		}
		if len(fields) < 3 { // not a key line: ssh skips these too
			continue
		}
		e.Hosts, e.KeyType, e.Key = strings.Split(fields[0], ","), fields[1], fields[2]
		out = append(out, e)
	}
	return out, nil
}

// DeleteKnownHosts rewrites the file without the lines in drop, 1-based,
// keeping the comments and the lines that hold no key.
func DeleteKnownHosts(drop map[int]bool) error {
	path := filepath.Join(Dir(), KnownHostsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	for i, line := range lines {
		if !drop[i+1] {
			kept = append(kept, line)
		}
	}
	return writeFileAtomic(path, []byte(strings.Join(kept, "\n")))
}
