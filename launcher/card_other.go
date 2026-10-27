//go:build !windows

package launcher

import "errors"

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

// OpenFolder: cards only exist on Windows.
func OpenFolder(string) error { return nil }
