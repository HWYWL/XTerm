package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DangerRulesFile is the file under the config directory that keeps the
// danger rules (danger.go in internal/app edits it).
const DangerRulesFile = "danger_rules.json"

// The two levels of a rule: one about what loses data or takes the machine
// down, and one about what only interrupts what runs. The window asks for the
// first to be typed out, and for the second to be confirmed once.
const (
	DangerHigh = "high"
	DangerHint = "hint"
)

// DangerRule is one rule: the regular expression a command is matched
// against, and what kind of command it catches. The rules are judged in the
// order they are listed.
type DangerRule struct {
	// ID is the app's own rule this one is, "" for one the user added: the
	// window names and explains it from the ID, in the language shown.
	ID string `json:"id,omitempty"`
	// Pattern is the regular expression, in RE2's syntax.
	Pattern string `json:"pattern"`
	// Name and Note are the user's, for a rule they added.
	Name string `json:"name,omitempty"`
	Note string `json:"note,omitempty"`
	// Level is DangerHint for a rule about an interruption; "" and
	// DangerHigh are the ones about data lost.
	Level string `json:"level,omitempty"`
	// LevelCustom is a level the user set in the window: a rule of the app's
	// keeps it when the app's own rules change under it.
	LevelCustom bool `json:"levelCustom,omitempty"`
	// Off is a rule the window switched off: it matches nothing.
	Off bool `json:"off,omitempty"`
}

// DefaultDangerRules are the rules a new installation starts with, in the
// order they are judged: the ones that lose data first, the hints after. The
// set is the original's, and the order is its too.
func DefaultDangerRules() []DangerRule {
	return []DangerRule{
		{ID: "rm-root", Level: DangerHigh,
			Pattern: `(^|[;&|]\s*)rm\s+(-[a-zA-Z]+\s+)*(/|/\*|--no-preserve-root\s+/)(\s|$)`},
		{ID: "rm-home", Level: DangerHigh,
			Pattern: `(^|[;&|]\s*)rm\s+(-[a-zA-Z]+\s+)*[~](/|\s|$)|\brm\s+-[a-zA-Z]*r[a-zA-Z]*\s+\$HOME(\s|/|$)`},
		{ID: "rm-cwd", Level: DangerHigh,
			Pattern: `(^|[;&|]\s*)rm\s+-[a-zA-Z]*[rf][a-zA-Z]*\s+\.(/)?\*?(\s|$)`},
		{ID: "disk-wipe", Level: DangerHigh,
			Pattern: `\bdd\b[^\n]*\bof=/dev/|\bmkfs(\.\w+)?\b|\b(shred|wipefs)\b[^\n]*/dev/|>\s*/dev/(sd|nvme|vd|hd|mmcblk)`},
		{ID: "fork-bomb", Level: DangerHigh,
			Pattern: `:\s*\(\s*\)\s*\{.*:\s*\|\s*:.*\}`},
		{ID: "overwrite-etc", Level: DangerHigh,
			Pattern: `>\s*/etc/(passwd|shadow|fstab|sudoers|hosts)\b|>\s*/boot/`},
		{ID: "shutdown", Level: DangerHint,
			Pattern: `(^|[;&|]\s*)(shutdown|reboot|halt|poweroff)\b|(^|[;&|]\s*)init\s+0\b|systemctl\s+(poweroff|reboot|halt)`},
		{ID: "chmod-root", Level: DangerHint,
			Pattern: `\bchmod\s+-[a-zA-Z]*R[a-zA-Z]*\s+[0-7]{3,4}\s+/(\s|$)|\bchown\s+-[a-zA-Z]*R[a-zA-Z]*\s+\S+\s+/(\s|$)`},
		{ID: "drop-db", Level: DangerHint,
			Pattern: `(?i)\bdrop\s+(database|table|schema)\b|\btruncate\s+table\b|\bdelete\s+from\s+\w+\s*;\s*$`},
		{ID: "redis-flush", Level: DangerHint,
			Pattern: `(?i)\bflushall\b|\bflushdb\b`},
		{ID: "container-prune", Level: DangerHint,
			Pattern: `(?i)docker\s+(system|volume|image|container)\s+prune|\bkubectl\s+delete\b`},
		{ID: "git-force", Level: DangerHint,
			Pattern: `(?i)git\s+push\b[^\n]*(\s--force\b|\s-f\b)|\bgit\s+reset\s+--hard\b|\bgit\s+clean\s+-[a-zA-Z]*[df]`},
		{ID: "curl-pipe-sh", Level: DangerHint,
			Pattern: `\b(curl|wget)\b[^\n|]*\|\s*(sudo\s+)?(ba|z|da)?sh\b`},
		{ID: "firewall-off", Level: DangerHint,
			Pattern: `\biptables\s+(-F|--flush)\b|\bufw\s+disable\b|\bsetenforce\s+0\b|\bsystemctl\s+(stop|disable)\s+(firewalld|ufw)\b`},
	}
}

// dangerAlias is the ID a rule is known by now: the first set of rules named
// two of them otherwise, and a file written then keeps its switches.
var dangerAlias = map[string]string{
	"rm-disk":   "disk-wipe",
	"overwrite": "overwrite-etc",
}

// LoadDangerRules reads the rules, the defaults for a missing file, and the
// defaults with what the user chose put back in front of the rules they added:
// a rule the app ships is named and explained by the app, in the language
// shown, and one it drops is not kept.
func LoadDangerRules() []DangerRule {
	data, err := os.ReadFile(filepath.Join(Dir(), DangerRulesFile))
	if err != nil {
		return DefaultDangerRules()
	}
	var saved []DangerRule
	if json.Unmarshal(data, &saved) != nil {
		return DefaultDangerRules()
	}
	return mergedDangerRules(saved)
}

// mergedDangerRules puts the app's rules in front, with what the user chose
// for each of them still theirs, and the rules they added after.
func mergedDangerRules(saved []DangerRule) []DangerRule {
	byID := map[string]DangerRule{}
	var custom []DangerRule
	for _, r := range saved {
		if id, ok := dangerAlias[r.ID]; ok {
			r.ID = id
		}
		if r.ID == "" {
			custom = append(custom, r)
			continue
		}
		byID[r.ID] = r
	}
	rules := DefaultDangerRules()
	for i := range rules {
		old, ok := byID[rules[i].ID]
		if !ok {
			continue
		}
		rules[i].Off = old.Off
		if old.LevelCustom && old.Level != "" {
			rules[i].Level = old.Level
			rules[i].LevelCustom = true
		}
	}
	return append(rules, custom...)
}

// SaveDangerRules writes the rules, and no file at all for the defaults: a
// new default then reaches the user who never changed them.
func SaveDangerRules(rules []DangerRule) error {
	if sameDangerRules(rules, DefaultDangerRules()) {
		err := os.Remove(filepath.Join(Dir(), DangerRulesFile))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(Dir(), DangerRulesFile), append(data, '\n'))
}

func sameDangerRules(a, b []DangerRule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
