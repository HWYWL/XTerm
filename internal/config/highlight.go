package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// HighlightRule is a rule of the terminal's text highlighting: a regular
// expression, and the color its matches take, as QuickTerm's settings have
// them. The rules are the user's, kept here whether or not the terminal
// can color the cells yet.
type HighlightRule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Pattern    string `json:"pattern"`
	Color      string `json:"color"`
	IgnoreCase bool   `json:"ignoreCase,omitempty"`
	Enabled    bool   `json:"enabled"`
	// Builtin marks a preset: its name, pattern and color come from the
	// app, and only its switch is the user's.
	Builtin bool `json:"builtin,omitempty"`
}

// highlightPath is where the rules are kept, beside the settings.
func highlightPath() string { return filepath.Join(Dir(), "highlight.json") }

// DefaultHighlightRules are the presets, in the order they are tried: the
// particular ones first, so that the number rule, which matches almost
// every line, does not take what a file size, a time or an address is.
func DefaultHighlightRules() []HighlightRule {
	ip := `(\d+\.){3}\d+(:\d+)?`
	perm := `(d|-|l|b|c|p|s)((r|-)(w|-)(x|-)){3}`
	ts := `(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2}\s+(\d{4}|\d{2}:\d{2}(:\d{2})?)` +
		`|\d{2}:\d{2}(:\d{2})?` +
		`|\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`
	size := `\b\d+(\.\d+)?[KMGTP]i?B?\b`
	url := "https?:[/]{2}[^\\s\"\\x27!*(){}|\\\\\\^<>`]*[^\\s\"\\x27:,.!?{}|\\\\\\^~\\[\\]`()<>]"
	kw := `\b(if|then|else|elif|fi|case|esac|for|select|while|until|do|done|in|function|time|coproc)\b`
	builtin := `\b(echo|ls|cd|pwd|touch|mkdir|rm|rmdir|mv|cp|cat|more|less|head|tail|grep` +
		`|find|man|info|whatis|whereis|which|type|help)\b`
	num := `\b\d+(\.\d+)?\b`
	return []HighlightRule{
		{ID: "hl-error", Name: "ERROR log", Pattern: `\b(?:ERROR|FATAL|CRITICAL|FAIL(?:ED|URE)?)\b`, Color: "#FF5555", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-warn", Name: "WARN log", Pattern: `\bWARN(?:ING)?\b`, Color: "#E5C07B", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-info", Name: "INFO log", Pattern: `\bINFO\b`, Color: "#3FB950", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-debug", Name: "DEBUG log", Pattern: `\bDEBUG\b`, Color: "#8B949E", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-perm", Name: "File permissions", Pattern: perm, Color: "#56D4DD", Enabled: true, Builtin: true},
		{ID: "hl-ts", Name: "Time", Pattern: ts, Color: "#BC8CFF", Enabled: true, Builtin: true},
		{ID: "hl-ipv4", Name: "IP address and port", Pattern: ip, Color: "#58A6FF", Enabled: true, Builtin: true},
		{ID: "hl-url", Name: "Link", Pattern: url, Color: "#A5D6FF", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-size", Name: "File size", Pattern: size, Color: "#F0883E", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-kw", Name: "Shell keywords", Pattern: kw, Color: "#F778BA", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-builtin", Name: "Built-in commands", Pattern: builtin, Color: "#7EE787", IgnoreCase: true, Enabled: true, Builtin: true},
		{ID: "hl-num", Name: "Number", Pattern: num, Color: "#79C0FF", Enabled: true, Builtin: true},
	}
}

// LoadHighlightRules reads the rules: the presets of this version, with
// the switches the user set on them, then the custom rules as saved.
//
// The presets' patterns, names and colors come from this version, as
// QuickTerm replaces its own on a version change: a preset that improved
// reaches an install that saved the old one. Only the switch is the
// user's to keep.
func LoadHighlightRules() []HighlightRule {
	var saved []HighlightRule
	if data, err := os.ReadFile(highlightPath()); err == nil {
		json.Unmarshal(data, &saved)
	}
	if len(saved) == 0 {
		return DefaultHighlightRules()
	}
	on := map[string]bool{}
	var custom []HighlightRule
	for _, r := range saved {
		if r.Builtin {
			on[r.ID] = r.Enabled
		} else {
			custom = append(custom, r)
		}
	}
	rules := DefaultHighlightRules()
	for i := range rules {
		if enabled, ok := on[rules[i].ID]; ok {
			rules[i].Enabled = enabled
		}
	}
	return append(rules, custom...)
}

// SaveHighlightRules writes them.
func SaveHighlightRules(rules []HighlightRule) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(highlightPath(), data, 0o600)
}
