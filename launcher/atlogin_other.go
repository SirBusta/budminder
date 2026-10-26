//go:build !windows

package launcher

// StartWithWindows: nothing on other systems (nil hides the menu item).
func StartWithWindows(string) *AtLogin { return nil }
