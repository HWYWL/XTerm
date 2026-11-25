package app

import (
	"strconv"
	"testing"

	"finallyshell/locales"
)

// TestProcDialog: a process's details open as a dialog, as QuickTerm has
// them: its numbers, its command line, its paths and the buttons that act
// on it; closing it leaves nothing behind.
func TestProcDialog(t *testing.T) {
	a, p, s, _, tt := processesApp(t)
	view := s.procs.shown()
	a.openProcDialog(s, view[0])
	p.until(t, "the details", func() bool {
		d := a.processes.detail
		return d != nil && !d.tab.loading
	})
	tt.Frame()
	for _, want := range []string{
		locales.ProcDialogTitle(), locales.ProcDialogSection(), locales.ProcDialogCommand(),
		locales.ProcDialogPaths(), locales.ProcDialogActions(), strconv.Itoa(view[0].PID),
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
	saveShot(t, tt, "process-dialog-en.png")

	a.closeProcDialog()
	tt.Frame()
	if a.processes.detail != nil {
		t.Error("the dialog is still open")
	}
}
