//go:build windows

package launcher

import (
	"os/exec"
	"runtime"
	"strings"
	"time"

	"fyne.io/systray"
)

// RunTray puts the BudMinder icon by the clock. Left-click opens BudMinder;
// right-click shows the menu. The returned func removes the icon (call it
// when BudMinder stops). ok is false when there's no icon.
func RunTray(t Tray) (remove func(), ok bool) {
	plain, err := TrayIcon(t.Icon, false)
	if err != nil {
		t.failed("tray icon", err)
		return func() {}, false
	}
	framed, err := TrayIcon(t.Icon, true)
	if err != nil {
		framed = plain
	}
	small, _ := IconPNG(t.Icon, 32) // the phone window's title bar
	ready := make(chan struct{})
	go func() {
		// The icon's window and its messages must stay on one thread.
		runtime.LockOSThread()
		systray.Run(func() { trayMenu(t, plain, framed, small); close(ready) }, nil)
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second): // no taskbar (a service, a locked-down PC)
		return func() {}, false
	}
	return systray.Quit, true
}

func trayMenu(t Tray, plain, framed, small []byte) {
	systray.SetIcon(plain)
	systray.SetTooltip(t.Tooltip(""))
	systray.SetOnTapped(func() { t.failed("open BudMinder", OpenBrowser(t.URL())) })

	title := systray.AddMenuItem("BudMinder "+t.Version, "")
	title.Disable()
	systray.AddSeparator()
	open := systray.AddMenuItem("Open BudMinder", "Open BudMinder in your browser")
	phone := systray.AddMenuItem("Open on your phone…", "Show a code to scan with your phone's camera")
	copyAddr := systray.AddMenuItem("Copy phone address", "Copy the address a phone on your Wi-Fi uses")
	systray.AddSeparator()
	update := systray.AddMenuItem("Update BudMinder", "")
	update.Hide()
	folder := systray.AddMenuItem("Open BudMinder folder", "Your data, photos and backups")
	var atLogin *systray.MenuItem
	if t.AtLogin != nil {
		atLogin = systray.AddMenuItemCheckbox("Start with Windows", "Start BudMinder when you sign in to Windows, so sensors keep recording", t.AtLogin.On())
	}
	systray.AddSeparator()
	stop := systray.AddMenuItem("Stop BudMinder", "Stop BudMinder (sensors stop recording until it starts again)")

	var atLoginClicked <-chan struct{} // nil: never
	if atLogin != nil {
		atLoginClicked = atLogin.ClickedCh
	}
	shown := ""
	showUpdate := func() {
		v := ""
		if t.Update != nil {
			v = t.Update()
		}
		if v == shown {
			return
		}
		shown = v
		systray.SetTooltip(t.Tooltip(v))
		if v == "" {
			systray.SetIcon(plain)
			update.Hide()
			return
		}
		systray.SetIcon(framed)
		update.SetTitle("Update to " + v + "…")
		update.Show()
	}
	showUpdate()
	tick := time.NewTicker(time.Minute)

	go func() {
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				showUpdate()
			case <-open.ClickedCh:
				t.failed("open BudMinder", OpenBrowser(t.URL()))
			case <-phone.ClickedCh:
				ShowPhoneCode(PhoneCode{Address: t.HomeNetworkURL(), Icon: small, Glyphs: t.Glyphs, Exe: t.Exe})
			case <-copyAddr.ClickedCh:
				if u := t.HomeNetworkURL(); u != "" {
					t.failed("copy the phone address", copyText(u))
				} else {
					ShowPhoneCode(PhoneCode{Icon: small, Glyphs: t.Glyphs, Exe: t.Exe}) // it explains why there's none
				}
			case <-update.ClickedCh:
				t.failed("open the update window", OpenBrowser(t.UpdateURL()))
			case <-folder.ClickedCh:
				t.failed("open the folder", OpenFolder(t.Folder))
			case <-atLoginClicked:
				on := !atLogin.Checked()
				if err := t.AtLogin.Set(on); err != nil {
					t.failed("change Start with Windows", err)
				}
				if t.AtLogin.On() {
					atLogin.Check()
				} else {
					atLogin.Uncheck()
				}
			case <-stop.ClickedCh:
				t.Stop()
				return
			}
		}
	}()
}

// copyText puts s on the clipboard with Windows' own clip command.
func copyText(s string) error {
	cmd := exec.Command("clip")
	cmd.Stdin = strings.NewReader(s)
	hideWindow(cmd)
	return cmd.Run()
}
