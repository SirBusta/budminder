//go:build !windows

package launcher

import (
	"errors"
	"runtime"
)

// Cards are drawn on Windows only; other systems keep the terminal.

// CardEvents is what a card's buttons, check box, link and close do.
type CardEvents struct {
	Button func(cw *CardWindow, id int) bool
	Check  func(cw *CardWindow, id int, on bool)
	Close  func(cw *CardWindow)
}

// CardWindow is a card on the screen (never, here).
type CardWindow struct{}

// SetLook gives the cards their icon and icon font.
func SetLook(icon256, glyphs []byte) {}

// RunCard shows nothing here and returns 0.
func RunCard(Card, CardEvents) int { return 0 }

// RunWelcome shows nothing here and returns 0.
func RunWelcome(Card, CardEvents) int { return 0 }

func (cw *CardWindow) SetLabel(int, string) {}
func (cw *CardWindow) Close(int)            {}

// SendToTray: no tray icon here.
func SendToTray(*CardWindow, bool) {}

// CopyText: cards only exist on Windows.
func CopyText(string) error { return errors.New("no clipboard here") }

// OpenFolder opens dir in the file manager: xdg-open on Linux (KDE, GNOME
// and most desktops have it), open on macOS. Settings > Photos uses it for
// the photo backup folder; it fails where there's no desktop.
func OpenFolder(dir string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := Outside(name, dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // xdg-open returns once the file manager has it
	return nil
}
