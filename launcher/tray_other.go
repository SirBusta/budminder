//go:build !windows

package launcher

// RunTray: no icon by the clock on this system yet (Linux and macOS are
// for later).
func RunTray(Tray) (remove func(), ok bool) { return func() {}, false }
