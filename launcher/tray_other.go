//go:build !windows

package launcher

// RunTray: no icon by the clock on this system yet (Linux and macOS are
// for later).
func RunTray(Tray) (remove func(), ok bool) { return func() {}, false }

// PhoneCode is what the phone window shows (Windows only).
type PhoneCode struct {
	Address string
	Icon    []byte
	Glyphs  []byte
	Exe     string
}

// ShowPhoneCode: no phone window on this system.
func ShowPhoneCode(PhoneCode) {}
