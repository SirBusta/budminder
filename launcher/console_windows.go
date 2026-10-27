//go:build windows

package launcher

import (
	"os"

	"golang.org/x/sys/windows"
)

// The Windows download is built without a console (-H windowsgui), so a
// double-click opens no black window: BudMinder talks through cards instead.
// Started from a terminal, it borrows that terminal for its messages and the
// welcome box, as before.

var (
	pAttachConsole    = kernel32.NewProc("AttachConsole")
	pGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
)

// AttachParentConsole: a program without a console started from a terminal
// writes to that terminal. Call first thing.
func AttachParentConsole() {
	if forcedWindowless() {
		return
	}
	if h, _, _ := pGetConsoleWindow.Call(); h != 0 {
		return // a console of its own (a development build)
	}
	if r, _, _ := pAttachConsole.Call(uintptr(^uint32(0))); r == 0 { // ATTACH_PARENT_PROCESS
		return // started from Explorer, an icon or another program
	}
	if out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout, os.Stderr = out, out
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(out.Fd()))
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(out.Fd()))
	}
	if in, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		os.Stdin = in
		_ = windows.SetStdHandle(windows.STD_INPUT_HANDLE, windows.Handle(in.Fd()))
	}
}

// Windowless: no console to write to, so messages go in cards.
func Windowless() bool {
	if forcedWindowless() {
		return true
	}
	h, _, _ := pGetConsoleWindow.Call()
	return h == 0
}

// forcedWindowless: BUDMINDER_WINDOWLESS=1 makes a test copy started from a
// terminal behave like the double-clicked download (cards, no welcome box).
func forcedWindowless() bool { return os.Getenv("BUDMINDER_WINDOWLESS") == "1" }
