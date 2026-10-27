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
// Storage Sense, a tidy-up) delete it; and in a synced folder (OneDrive,
// Dropbox, Google Drive, iCloud Drive, Box) the syncing app copies the
// database while BudMinder is writing to it, which can damage it.

// RiskyFolder names why dir is a bad home for BudMinder: "Downloads", the
// syncing app ("OneDrive", "Dropbox", "Google Drive", "iCloud Drive",
// "Box"; see Synced), or "" when it's fine.
func RiskyFolder(dir string) string {
	clean := filepath.Clean(dir)
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if name := riskyName(strings.ToLower(part)); name != "" {
			return name
		}
	}
	for _, v := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
		if root := os.Getenv(v); root != "" && within(clean, root) {
			return "OneDrive"
		}
	}
	// Box Drive's folder is plain "Box" in the home folder; a "box" elsewhere
	// is likely just a grow box.
	if home, err := os.UserHomeDir(); err == nil && within(clean, filepath.Join(home, "Box")) {
		return "Box"
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

// riskyName: one lower-case folder name in a path. The macOS names are
// the folders under ~/Library/CloudStorage and iCloud's Mobile Documents.
func riskyName(p string) string {
	switch {
	case p == "downloads":
		return "Downloads"
	case p == "onedrive" || strings.HasPrefix(p, "onedrive - ") || strings.HasPrefix(p, "onedrive-"):
		return "OneDrive"
	case p == "dropbox" || strings.HasPrefix(p, "dropbox (") || strings.HasPrefix(p, "dropbox-"):
		return "Dropbox"
	case p == "google drive" || p == "googledrive" || p == "my drive" || p == "shared drives" || strings.HasPrefix(p, "googledrive-"):
		return "Google Drive"
	case p == "icloud drive" || p == "iclouddrive" || p == "com~apple~clouddocs":
		return "iCloud Drive"
	case strings.HasPrefix(p, "box-box"):
		return "Box"
	}
	return ""
}

// Synced: risky (from RiskyFolder) is a syncing app, not Downloads.
func Synced(risky string) bool { return risky != "" && risky != "Downloads" }

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
