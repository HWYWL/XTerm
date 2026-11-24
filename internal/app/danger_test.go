package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The danger rules window: the app's rules are named, explained, tagged high
// or hint, and switched on and off, and the assets window's risky count
// follows them, as does the file they are kept in.
func TestDangerRules(t *testing.T) {
	a, p := testApp(t)
	if !a.risky("rm -rf /") || a.risky("rm -rf ./build") || a.risky("ls -l") {
		t.Errorf("defaults: rm %v, build %v, ls %v",
			a.risky("rm -rf /"), a.risky("rm -rf ./build"), a.risky("ls -l"))
	}

	a.openPlugin(pluginDanger)
	w := p.show(t, WindowDangerRules)
	w.Frame()
	all := len(config.DefaultDangerRules())
	if all != 14 {
		t.Errorf("the app has %d rules", all)
	}
	for _, s := range []string{locales.DangerNote(), locales.DangerAdd(), locales.DangerRestore(),
		locales.DangerAllOff(), locales.DangerOn(all, all), locales.DangerHigh(), locales.DangerHint(),
		locales.DangerRuleRmRoot(), locales.DangerRuleRmRootNote(), locales.DangerRuleFirewallOff()} {
		if !w.HasText(s) {
			t.Errorf("no %q; texts %q", s, w.Texts())
		}
	}

	// A rule of the user's applies from then on, and is kept.
	a.danger.draft = `(?i)terraform\s+destroy`
	if err := w.Click(locales.DangerAdd()); err != nil {
		t.Fatal(err)
	}
	if !a.risky("terraform destroy -auto-approve") || a.danger.err != "" {
		t.Errorf("added: risky %v, err %q", a.risky("terraform destroy -auto-approve"), a.danger.err)
	}
	if rules := config.LoadDangerRules(); len(rules) != all+1 {
		t.Errorf("kept %+v", rules)
	}

	// What is not a regular expression is refused, and nothing changes.
	before := len(a.danger.rules)
	a.danger.draft = "("
	if err := w.Click(locales.DangerAdd()); err != nil {
		t.Fatal(err)
	}
	if a.danger.err != locales.DangerInvalid() || len(a.danger.rules) != before {
		t.Errorf("invalid: err %q, rules %d", a.danger.err, len(a.danger.rules))
	}

	// The tag flips the level of the rule it is on, here the first, and the
	// file keeps it as the user's.
	w.Frame()
	if err := w.Click(locales.DangerHigh()); err != nil {
		t.Fatal(err)
	}
	if a.danger.rules[0].Level != config.DangerHint || !a.danger.rules[0].LevelCustom {
		t.Errorf("level %q, custom %v", a.danger.rules[0].Level, a.danger.rules[0].LevelCustom)
	}
	if rules := config.LoadDangerRules(); rules[0].Level != config.DangerHint {
		t.Errorf("kept level %q", rules[0].Level)
	}

	// One rule switched off stops counting on its own, and the count says so.
	a.setDangerOff(0, true)
	if !a.danger.rules[0].Off {
		t.Error("the first rule stayed on")
	}
	w.Frame()
	if !w.HasText(locales.DangerOn(all, all+1)) {
		t.Errorf("the count did not follow; texts %q", w.Texts())
	}
	a.setDangerOff(0, false)

	// Every rule off leaves nothing risky, and the button turns them back on.
	w.Frame()
	if err := w.Click(locales.DangerAllOff()); err != nil {
		t.Fatal(err)
	}
	if a.risky("rm -rf /") || a.risky("terraform destroy") || a.risky("shutdown -h now") {
		t.Error("a rule stayed on after turning them all off")
	}
	w.Frame()
	if !w.HasText(locales.DangerOn(0, all+1)) || !w.HasText(locales.DangerAllOn()) {
		t.Errorf("all off: texts %q", w.Texts())
	}
	if err := w.Click(locales.DangerAllOn()); err != nil {
		t.Fatal(err)
	}
	if !a.risky("rm -rf /") {
		t.Error("the rules did not come back on")
	}

	// Removing a rule takes it out of the count, and restoring the defaults
	// takes the user's rules away.
	w.Frame()
	if err := w.Click(locales.Delete()); err != nil {
		t.Fatal(err)
	}
	if a.danger.rules[0].ID != "rm-home" {
		t.Errorf("the first rule is %q", a.danger.rules[0].ID)
	}
	w.Frame()
	if err := w.Click(locales.DangerRestore()); err != nil {
		t.Fatal(err)
	}
	if !a.risky("rm -rf /") || a.risky("terraform destroy") {
		t.Error("the defaults did not come back clean")
	}
	if rules := config.LoadDangerRules(); len(rules) != all {
		t.Errorf("kept %+v", rules)
	}
}

// The guard holds the Enter that would run a command the rules catch: a hint
// asks once, a high rule asks for its name, a cancel clears the line, and the
// plugin's switch turns the guard off with the rules.
func TestDangerGuard(t *testing.T) {
	a, p := testApp(t)
	if a.pluginOn(pluginDanger) != true {
		t.Fatal("the danger plugin starts off")
	}

	// A hint rules holds the Enter, and runs the command once confirmed.
	s := &Session{cmdLine: "shutdown -h now"}
	if !a.dangerKey(s, nil, 0, ui.KeyEnter) {
		t.Fatal("the Enter of a dangerous command went through")
	}
	if s.cmdLine != "" {
		t.Errorf("the line is still %q", s.cmdLine)
	}
	w := p.show(t, WindowDangerGuard)
	w.Frame()
	if !w.HasText(locales.DangerRuleShutdown()) || !w.HasText(locales.DangerGuardRunAnyway()) {
		t.Errorf("the guard's window: %q", w.Texts())
	}
	if err := w.Click(locales.DangerGuardRunAnyway()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowDangerGuard) || a.assetRun.cmd != 1 {
		t.Errorf("after a yes: open %v, counted %d", a.windowOpen(WindowDangerGuard), a.assetRun.cmd)
	}

	// A high rule asks for its name, and the button waits for it.
	s.cmdLine = "rm -rf /"
	if !a.dangerKey(s, nil, 0, ui.KeyEnter) {
		t.Fatal("the Enter of a high command went through")
	}
	w = p.show(t, WindowDangerGuard)
	w.Frame()
	if !w.HasText(locales.DangerGuardType(locales.DangerRuleRmRoot())) {
		t.Errorf("no ask for the name; texts %q", w.Texts())
	}
	_ = w.Click(locales.DangerGuardRun())
	if a.assetRun.cmd != 1 {
		t.Errorf("a command ran without its name typed")
	}
	a.guard.typed = locales.DangerRuleRmRoot()
	w.Frame()
	if err := w.Click(locales.DangerGuardRun()); err != nil {
		t.Fatal(err)
	}
	if a.assetRun.cmd != 2 {
		t.Errorf("counted %d after the name", a.assetRun.cmd)
	}

	// A cancel runs nothing and takes the line back off the shell.
	s.cmdLine = "rm -rf /"
	if !a.dangerKey(s, nil, 0, ui.KeyEnter) {
		t.Fatal("the Enter went through before the cancel")
	}
	w.Frame()
	if err := w.Click(locales.DangerGuardCancel()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowDangerGuard) || a.assetRun.cmd != 2 {
		t.Errorf("after a no: open %v, counted %d", a.windowOpen(WindowDangerGuard), a.assetRun.cmd)
	}

	// Off in the plugin center, nothing is held.
	a.setPlugin(pluginDanger, false)
	s.cmdLine = "rm -rf /"
	if a.dangerKey(s, nil, 0, ui.KeyEnter) {
		t.Error("the Enter was held with the plugin off")
	}
}

// A line the arrows went through is asked about once, as a hint, where it
// still reads like something destructive.
func TestDangerGuardEditedLine(t *testing.T) {
	a, _ := testApp(t)
	s := &Session{cmdLine: "rm -rf *", cmdEdited: true}
	if m := a.dangerHit(s.cmdLine, s.cmdEdited); m == nil || dangerHigh(m.rule) {
		t.Errorf("an edited line: %+v", m)
	}
	if m := a.dangerHit("ls", true); m != nil {
		t.Errorf("an edited ls was asked about: %+v", m)
	}
	if m := a.dangerHit("rm -rf /", false); m == nil || m.rule.ID != "rm-root" {
		t.Errorf("a whole line: %+v", m)
	}
}
