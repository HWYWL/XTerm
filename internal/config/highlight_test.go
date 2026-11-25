package config

import (
	"regexp"
	"testing"
)

// TestDefaultHighlightRules: the presets compile, and the number rule,
// which matches almost any line, comes last, as the others then take the
// digits of a time or a size first.
func TestDefaultHighlightRules(t *testing.T) {
	rules := DefaultHighlightRules()
	if len(rules) != 12 {
		t.Fatalf("%d presets, want 12", len(rules))
	}
	for _, r := range rules {
		if !r.Builtin || !r.Enabled || r.ID == "" || r.Name == "" {
			t.Errorf("preset %+v", r)
		}
		pattern := r.Pattern
		if r.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		if _, err := regexp.Compile(pattern); err != nil {
			t.Errorf("%s: %v", r.ID, err)
		}
	}
	if last := rules[len(rules)-1]; last.ID != "hl-num" {
		t.Errorf("last rule is %q, want the number rule", last.ID)
	}
}

// TestHighlightRulesSaved: the presets come from the app, keeping the
// switches the user set, and the custom rules as saved.
func TestHighlightRulesSaved(t *testing.T) {
	t.Setenv("FINALLYSHELL_DIR", t.TempDir())
	if got := LoadHighlightRules(); len(got) != len(DefaultHighlightRules()) {
		t.Fatalf("%d rules with none saved, want the presets", len(got))
	}

	saved := DefaultHighlightRules()
	saved[0].Enabled = false
	saved[0].Pattern = "an old pattern"
	saved = append(saved, HighlightRule{ID: "mine", Name: "Mine", Pattern: `NXDOMAIN`, Color: "#FF5555", Enabled: true})
	if err := SaveHighlightRules(saved); err != nil {
		t.Fatal(err)
	}

	got := LoadHighlightRules()
	if len(got) != 13 {
		t.Fatalf("%d rules, want the 12 presets and the custom one", len(got))
	}
	if got[0].Enabled {
		t.Error("the preset's switch was not kept")
	}
	if got[0].Pattern == "an old pattern" {
		t.Error("the preset's own pattern was kept")
	}
	if last := got[12]; last.ID != "mine" || last.Builtin || last.Name != "Mine" {
		t.Errorf("custom rule %+v", last)
	}
}
