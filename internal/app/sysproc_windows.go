//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a program the app runs from opening a console window of
// its own, which a GUI program would flash over the screen.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
