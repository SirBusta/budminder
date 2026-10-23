//go:build !windows

package launcher

import (
	"errors"
	"os/exec"
)

func hideWindow(*exec.Cmd) {}

// StartHidden is only needed on Windows, where a program started from an
// icon would otherwise keep a window open; elsewhere the icon runs BudMinder
// directly, with no terminal.
func StartHidden(string, ...string) error {
	return errors.New("not needed on this system")
}
