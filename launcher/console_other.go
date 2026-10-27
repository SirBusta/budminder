//go:build !windows

package launcher

// AttachParentConsole: other systems always run in a terminal or as a service.
func AttachParentConsole() {}

// Windowless: never on other systems; they keep the terminal.
func Windowless() bool { return false }
