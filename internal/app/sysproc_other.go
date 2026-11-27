//go:build !windows

package app

import "os/exec"

// hideWindow is nothing where a program the app runs opens no window.
func hideWindow(cmd *exec.Cmd) {}
