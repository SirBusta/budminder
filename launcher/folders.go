package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BudMinder keeps its data next to the program, so a program left in
// Downloads keeps the grow journal there too, where clean-ups (Windows'
// Storage Sense, a tidy-up) delete it; and in OneDrive, syncing a database
// that's in use can damage it.

// RiskyFolder names why dir is a bad home for BudMinder: "Downloads",
// "OneDrive", or "" when it's fine.
func RiskyFolder(dir string) string {
	clean := filepath.Clean(dir)
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		p := strings.ToLower(part)
		if p == "downloads" {
			return "Downloads"
		}
		if p == "onedrive" || strings.HasPrefix(p, "onedrive - ") {
			return "OneDrive"
		}
	}
	for _, v := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
		if root := os.Getenv(v); root != "" && within(clean, root) {
			return "OneDrive"
		}
	}
	// A translated Downloads folder on Linux (~/Téléchargements, ...).
	if runtime.GOOS == "linux" {
		if out, err := exec.Command("xdg-user-dir", "DOWNLOAD").Output(); err == nil {
			if d := strings.TrimSpace(string(out)); d != "" && within(clean, d) {
				if home, _ := os.UserHomeDir(); filepath.Clean(d) != filepath.Clean(home) {
					return "Downloads"
				}
			}
		}
	}
	return ""
}

func within(dir, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SuggestedHome is a good folder for BudMinder on this system, to suggest
// when it's in a risky one.
func SuggestedHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "a folder of its own"
	}
	name := "BudMinder"
	if runtime.GOOS == "linux" {
		name = "budminder" // as in the README's Linux guide
	}
	return filepath.Join(home, name)
}
