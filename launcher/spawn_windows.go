//go:build windows

package launcher

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000 // CREATE_NO_WINDOW

// hideWindow keeps a helper command (PowerShell) from flashing a window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// StartHidden starts the program again with no window, detached from this
// one: how the icon runs BudMinder in the background on Windows.
func StartHidden(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	hideWindow(cmd)
	return cmd.Start()
}
