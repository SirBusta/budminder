package launcher

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
)

// The first run from Downloads or a synced folder (OneDrive, Dropbox, ...),
// before any data exists there:
// BudMinder explains that it keeps the grow journal next to the program, lets
// the user pick a folder, copies itself into a BudMinder folder there and
// starts from that copy. Cancel keeps it where it is, with the warning in the
// welcome box.

// InstalledFromFlag tells the copy where it was copied from, so its welcome
// box can say the download may be deleted. Not for people to type.
const InstalledFromFlag = "--installed-from"

const homeExplain = "BudMinder needs a home folder.\n\n" +
	"It keeps your grow journal (database and photos) next to the program, " +
	"so it shouldn't run from your %s folder, where %s.\n\n" +
	"Choose a folder for BudMinder. It will set itself up in a BudMinder folder there, " +
	"and you can delete the file you downloaded."

// ChooseHome asks where BudMinder should live. It returns the chosen folder,
// or "" when the user cancelled or there's no way to ask.
func ChooseHome(risky string) string {
	why := "clean-ups can delete it"
	if Synced(risky) {
		why = risky + " copies files to the cloud while BudMinder is still writing to them, which can damage the database"
	}
	text := fmt.Sprintf(homeExplain, risky, why)
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return chooseWindows(text, home)
	case "linux":
		return chooseLinux(text, home)
	}
	return "" // macOS: the warning in the welcome box, for now
}

func chooseWindows(text, start string) string {
	// A top-most owner window keeps both dialogs in front of the console.
	script := `Add-Type -AssemblyName System.Windows.Forms
$owner = New-Object System.Windows.Forms.Form -Property @{ TopMost = $true; ShowInTaskbar = $false }
$ok = [System.Windows.Forms.MessageBox]::Show($owner, $env:BM_TEXT, 'BudMinder', 'OKCancel', 'Information')
if ($ok -ne 'OK') { exit 0 }
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Choose where BudMinder should live. It makes a BudMinder folder inside the one you pick.'
$d.SelectedPath = $env:BM_START
$d.ShowNewFolderButton = $true
if ($d.ShowDialog($owner) -eq 'OK') { Write-Output $d.SelectedPath }
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), "BM_TEXT="+text, "BM_START="+start)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func chooseLinux(text, start string) string {
	if p, err := exec.LookPath("kdialog"); err == nil { // KDE (Kubuntu)
		if exec.Command(p, "--title", "BudMinder", "--yes-label", "Choose folder", "--no-label", "Cancel", "--yesno", text).Run() != nil {
			return ""
		}
		out, err := exec.Command(p, "--title", "Choose where BudMinder should live", "--getexistingdirectory", start).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	if p, err := exec.LookPath("zenity"); err == nil { // GNOME and others
		if exec.Command(p, "--question", "--title=BudMinder", "--ok-label=Choose folder", "--cancel-label=Cancel", "--width=420", "--text="+text).Run() != nil {
			return ""
		}
		out, err := exec.Command(p, "--file-selection", "--directory", "--title=Choose where BudMinder should live", "--filename="+start+"/").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	// No dialogs: ask in the terminal, when there is one.
	if !IsTerminal(os.Stdin) {
		return ""
	}
	fmt.Println("\n" + strings.ReplaceAll(text, "Choose a folder", "Type a folder"))
	fmt.Printf("\nFolder (press Enter for %s, or type cancel): ", start)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return typedFolder(line, start)
}

// typedFolder turns the folder typed at the prompt into a full path: Enter
// is start, ~/x and plain relative names sit under start (the home folder,
// not wherever the program was started), cancel is "".
func typedFolder(line, start string) string {
	line = strings.TrimSpace(line)
	switch strings.ToLower(line) {
	case "":
		return start
	case "cancel", "c", "n", "no":
		return ""
	}
	if strings.HasPrefix(line, "~/") {
		return filepath.Join(start, line[2:])
	}
	if !filepath.IsAbs(line) {
		return filepath.Join(start, line)
	}
	return line
}

// Install copies the program into a BudMinder folder inside chosen (or into
// chosen itself when that already is one) and returns the copy's path.
func Install(exe, chosen string) (string, error) {
	if chosen == "" {
		return "", errors.New("no folder chosen")
	}
	name := "BudMinder"
	if runtime.GOOS == "linux" {
		name = "budminder" // as in the README's Linux guide
	}
	dir := filepath.Clean(chosen)
	if !strings.EqualFold(filepath.Base(dir), name) {
		dir = filepath.Join(dir, name)
	}
	if RiskyFolder(dir) != "" {
		return "", fmt.Errorf("%s is inside %s too", dir, RiskyFolder(dir))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(exe))
	if err := copyFile(exe, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// RunInstalled starts the copy in this same window and waits for it, so the
// user keeps one window showing BudMinder's welcome box. Returns its exit code.
func RunInstalled(copyExe, from string) int {
	cmd := exec.Command(copyExe, InstalledFromFlag, from)
	cmd.Dir = filepath.Dir(copyExe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl+C reaches the copy too; this one waits for it to finish stopping.
	signal.Ignore(os.Interrupt)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Println("Couldn't start BudMinder from its new folder:", err)
		return 1
	}
	return 0
}
