package launcher

// Tray is what the icon by the clock needs from the running BudMinder
// (Windows only for now; other systems get no icon, see tray_other.go).
type Tray struct {
	Version string
	Port    string
	Folder  string // the program's folder (data, uploads, backups)
	Icon    []byte // the 256x256 PNG
	Glyphs  []byte // Font Awesome solid .ttf, for the phone window's icons
	Exe     string // this program (the phone window's Allow through firewall)

	// Update is the newer version to offer, or "" (checked every minute).
	Update func() string
	// AtLogin is nil when "Start with Windows" isn't offered (a test run or
	// a second copy, which must not take over the real install's start).
	AtLogin *AtLogin
	// Stop shuts BudMinder down, the same as Stop BudMinder in the Start menu.
	Stop func()
	// Failed hears about a menu item that didn't work (for the log); may be nil.
	Failed func(what string, err error)
}

func (t Tray) failed(what string, err error) {
	if err != nil && t.Failed != nil {
		t.Failed(what, err)
	}
}

// AtLogin turns "Start with Windows" on and off for this program.
type AtLogin struct {
	On  func() bool
	Set func(on bool) error
}

// URL is where this computer opens BudMinder.
func (t Tray) URL() string { return "http://localhost:" + t.Port }

// UpdateURL opens BudMinder with the Update available window already open.
func (t Tray) UpdateURL() string { return t.URL() + "/?update=1" }

// HomeNetworkURL is the address a phone on the same Wi-Fi uses, or "" when
// this computer isn't on a home network. Asked each time: it can change.
func (t Tray) HomeNetworkURL() string {
	ip := LANAddress()
	if ip == "" {
		return ""
	}
	return "http://" + ip + ":" + t.Port
}

// Tooltip is shown when pointing at the icon.
func (t Tray) Tooltip(update string) string {
	s := "BudMinder " + t.Version + " is running"
	if update != "" {
		s += "\nUpdate available: " + update
	}
	return s
}
