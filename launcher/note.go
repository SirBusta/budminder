package launcher

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The note: one small file in the user's settings folder saying where
// BudMinder is installed and which version it is. Every start rewrites it,
// so it follows a moved folder. A newer download double-clicked anywhere
// (usually Downloads) reads it and offers to update that install instead of
// setting up a new one; an older or the same download opens the install.
//
//	Windows  %AppData%\BudMinder\home.txt
//	Linux    ~/.config/budminder/home
//	macOS    ~/Library/Application Support/BudMinder/home

// UpdatedFromFlag tells the updated program where the download was, so its
// welcome box can say it may be deleted. Not for people to type.
const UpdatedFromFlag = "--updated-from"

// Note is what the note says.
type Note struct {
	Exe     string // the installed program
	Version string
}

// NotePath is where the note lives on this system.
func NotePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" {
		return filepath.Join(dir, "budminder", "home"), nil
	}
	name := "home"
	if runtime.GOOS == "windows" {
		name = "home.txt"
	}
	return filepath.Join(dir, "BudMinder", name), nil
}

// WriteNote records this program as the installed BudMinder.
func WriteNote(exe, version string) error {
	p, err := NotePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(exe+"\n"+version+"\n"), 0o644)
}

// ReadNote reads the note; ok is false when there's none.
func ReadNote() (Note, bool) {
	p, err := NotePath()
	if err != nil {
		return Note{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Note{}, false
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" {
		return Note{}, false
	}
	return Note{Exe: strings.TrimSpace(lines[0]), Version: strings.TrimSpace(lines[1])}, true
}

// What a download should do about an existing install.
type Plan int

const (
	PlanNone   Plan = iota // no other install: carry on as usual
	PlanUpdate             // the install is older: offer to update it
	PlanOpen               // the install is the same or newer: open it instead
)

// Decide what this program (exe, version, running in folder here) should
// do, given the note. It only acts when this program is somewhere else and
// has no data of its own, and the noted install really is there with its
// database.
func Decide(exe, here, version string, note Note, ok bool, newer func(a, b string) bool, exists func(string) bool) Plan {
	if !ok || note.Exe == "" || sameFile(exe, note.Exe) {
		return PlanNone
	}
	if exists(filepath.Join(here, "data", "isley.db")) {
		return PlanNone // this copy is an install of its own
	}
	if !exists(note.Exe) || !exists(filepath.Join(filepath.Dir(note.Exe), "data", "isley.db")) {
		return PlanNone // the noted install is gone
	}
	if newer(version, note.Version) {
		return PlanUpdate
	}
	return PlanOpen
}

func sameFile(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// ConfirmUpdate asks whether to update the install at dir from one version
// to another: a window on Windows and Linux desktops, the terminal otherwise.
func ConfirmUpdate(dir, from, to string) bool {
	text := fmt.Sprintf("Update BudMinder in %s from %s to %s?\n\n"+
		"Your database is backed up first, and the current version is kept as a fallback. "+
		"Afterwards you can delete the file you downloaded.", dir, from, to)
	switch runtime.GOOS {
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms
$owner = New-Object System.Windows.Forms.Form -Property @{ TopMost = $true; ShowInTaskbar = $false }
$r = [System.Windows.Forms.MessageBox]::Show($owner, $env:BM_TEXT, 'BudMinder', 'YesNo', 'Question')
if ($r -eq 'Yes') { Write-Output 'yes' }
`
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass", "-Command", script)
		cmd.Env = append(os.Environ(), "BM_TEXT="+text)
		hideWindow(cmd)
		out, err := cmd.Output()
		return err == nil && strings.TrimSpace(string(out)) == "yes"
	case "linux":
		if p, err := exec.LookPath("kdialog"); err == nil {
			return exec.Command(p, "--title", "BudMinder", "--yes-label", "Update", "--no-label", "Cancel", "--yesno", text).Run() == nil
		}
		if p, err := exec.LookPath("zenity"); err == nil {
			return exec.Command(p, "--question", "--title=BudMinder", "--ok-label=Update", "--cancel-label=Cancel", "--width=420", "--text="+text).Run() == nil
		}
	}
	if !IsTerminal(os.Stdin) {
		return false
	}
	fmt.Println("\n" + text)
	fmt.Print("\nUpdate now? [Y/n] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true
	}
	return false
}

// UpdateInstall stops the noted install if it's running, backs up its
// database with backup(db, copy) (main uses SQLite's VACUUM INTO, so the copy
// is one complete file), and puts this program (download) in its place,
// keeping the old one as exe.old.
func UpdateInstall(download string, note Note, to string, backup func(db, dst string) error) error {
	dir := filepath.Dir(note.Exe)
	data := filepath.Join(dir, "data")
	if err := Stop(data); err == nil {
		// Wait for it to finish shutting down (its stop line file goes away).
		for range 60 {
			if _, err := os.Stat(filepath.Join(data, runFile)); err != nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		time.Sleep(time.Second)
	}
	backups := filepath.Join(data, "backups")
	if err := os.MkdirAll(backups, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(backups, fmt.Sprintf("before-%s-%s.db", to, time.Now().Format("20060102-150405")))
	if err := backup(filepath.Join(data, "isley.db"), dst); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	old := note.Exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(note.Exe, old); err != nil {
		return errors.New("couldn't replace the program; is BudMinder still running? Stop it and try again")
	}
	if err := copyFile(download, note.Exe); err != nil {
		_ = os.Rename(old, note.Exe)
		return err
	}
	return nil
}

// RunUpdated starts the updated install in this window and waits for it.
func RunUpdated(exe, download string) int {
	cmd := exec.Command(exe, UpdatedFromFlag, download)
	cmd.Dir = filepath.Dir(exe)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt) // Ctrl+C reaches it too; this one waits for it
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Println("Couldn't start the updated BudMinder:", err)
		return 1
	}
	return 0
}

// OpenInstalled opens the noted install the way its icon does.
func OpenInstalled(exe string) error {
	cmd := exec.Command(exe, OpenFlag)
	cmd.Dir = filepath.Dir(exe)
	return cmd.Start()
}
