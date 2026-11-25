package app

import (
	"strconv"
	"testing"

	"finallyshell/locales"
)

// TestProcDialog: a process's details open as a dialog, as QuickTerm has
// them: its numbers, its causal chain, its command line, its paths and the
// buttons that act on it; closing it leaves nothing behind.
func TestProcDialog(t *testing.T) {
	a, p, s, _, tt := processesApp(t)
	view := s.procs.shown()
	a.openProcDialog(s, view[0])
	p.until(t, "the details and the chain", func() bool {
		d := a.processes.detail
		return d != nil && !d.tab.loading && !d.chainLoading
	})
	tt.Frame()
	for _, want := range []string{
		locales.ProcDialogTitle(), locales.ProcDialogSection(), locales.ProcDialogChain(),
		locales.ProcDialogCommand(), locales.ProcDialogPaths(), locales.ProcDialogActions(),
		strconv.Itoa(view[0].PID),
		// The chain, from init down: the login that started java.
		locales.ProcChainKindInit(), locales.ProcChainKindSSH(), locales.ProcChainKindMultiplexer(),
		locales.ProcChainWarnBigRSS("1884"), locales.ProcChainWarnSSH(), "/sbin/init splash",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
	saveShot(t, tt, "process-dialog-en.png")

	// A process with no chain read says why, instead of guessing.
	a.closeProcDialog()
	a.openProcDialog(s, s.procs.list[0]) // systemd
	p.until(t, "the reason", func() bool {
		d := a.processes.detail
		return d != nil && !d.chainLoading
	})
	tt.Frame()
	if !tt.HasText(locales.ProcChainReasonGone()) {
		t.Errorf("no reason; texts %q", tt.Texts())
	}

	a.closeProcDialog()
	tt.Frame()
	if a.processes.detail != nil {
		t.Error("the dialog is still open")
	}
	if d := a.processes.detail; d != nil {
		t.Errorf("left behind %+v", d)
	}
}
