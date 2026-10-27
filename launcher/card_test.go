package launcher

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestWords(t *testing.T) {
	t.Parallel()
	got := Words("Use **Stop BudMinder** in `C:\\Users\\you\\BudMinder`.")
	want := []Span{{"Use ", Plain}, {"Stop BudMinder", Bold}, {" in ", Plain}, {"C:\\Users\\you\\BudMinder", Code}, {".", Plain}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Words = %#v", got)
	}
	// ** inside a path is part of the path.
	if got := Words("`a**b`"); len(got) != 1 || got[0].Text != "a**b" || got[0].Style != Code {
		t.Errorf("code with stars = %#v", got)
	}
}

// cardText is everything a card says, for looking things up.
func cardText(c Card) string {
	var b strings.Builder
	b.WriteString(c.Title + "\n")
	for _, bl := range c.Blocks {
		b.WriteString(spansText(bl.Text) + "\n")
		for _, r := range bl.Rows {
			b.WriteString(spansText(r.Text) + "\n")
		}
		for _, bt := range bl.Buttons {
			b.WriteString("[" + bt.Label + "]\n")
		}
	}
	return b.String()
}

func findBlock(c Card, k BlockKind) *Block {
	for i := range c.Blocks {
		if c.Blocks[i].Kind == k {
			return &c.Blocks[i]
		}
	}
	return nil
}

func TestWelcomeCard_FirstStart(t *testing.T) {
	t.Parallel()
	c := WelcomeCard(Welcome{Port: "8080", Folder: `C:\Users\you\BudMinder`, Setup: true, IconsMade: true, IconDesktop: true, Tray: true, LANAddress: "192.168.1.5"}, "1.5.8")
	text := cardText(c)
	for _, s := range []string{
		"BudMinder is running",
		"Head there to set up your username and password.",
		"available in your Start menu alongside Stop BudMinder",
		"Put a BudMinder icon on the desktop",
		`stored in C:\Users\you\BudMinder. Do not delete this folder`,
		"BudMinder runs in the background",
		"Hidden icon?",
		"does not shut down BudMinder",
		"[Got it]",
		"Open BudMinder on your phone",
	} {
		if !strings.Contains(text, s) {
			t.Errorf("first start card lacks %q:\n%s", s, text)
		}
	}
	if b := findBlock(c, BlockCheck); b == nil || !b.Checked || b.ID != CardDesktop {
		t.Errorf("desktop box = %+v, want ticked", b)
	}
	if findBlock(c, BlockBanner) != nil {
		t.Error("a first start from its own folder has no banner")
	}
	// The tray row shows the BudMinder icon, not a glyph.
	rows := c.Blocks
	found := false
	for _, b := range rows {
		for _, r := range b.Rows {
			found = found || (r.AppIcon && strings.Contains(spansText(r.Text), "green leaf icon"))
		}
	}
	if !found {
		t.Error("the system tray row should carry the BudMinder icon")
	}
}

func TestWelcomeCard_Variants(t *testing.T) {
	t.Parallel()
	moved := WelcomeCard(Welcome{Folder: `C:\Users\you\BudMinder`, Installed: `C:\Users\you\Downloads\b.exe`, IconsMade: true}, "1.5.8")
	if b := findBlock(moved, BlockBanner); b == nil || b.Warn || !strings.Contains(spansText(b.Text), "set itself up in") {
		t.Errorf("moved banner = %+v", b)
	}
	stayed := WelcomeCard(Welcome{Folder: `C:\Users\you\Downloads`, Risky: "Downloads", Suggest: `C:\Users\you\BudMinder`}, "1.5.8")
	if b := findBlock(stayed, BlockBanner); b == nil || !b.Warn || !strings.Contains(spansText(b.Text), "Downloads") {
		t.Errorf("Downloads banner = %+v", b)
	}
	if findBlock(stayed, BlockCheck) != nil {
		t.Error("no icons are made in Downloads, so no desktop box")
	}
	synced := WelcomeCard(Welcome{Folder: `C:\Users\you\Dropbox\BudMinder`, Risky: "Dropbox", Suggest: `C:\Users\you\BudMinder`}, "1.5.8")
	if b := findBlock(synced, BlockBanner); b == nil || !b.Warn || !strings.Contains(spansText(b.Text), "Dropbox copies files to the cloud while BudMinder is still writing to them") {
		t.Errorf("Dropbox banner = %+v", b)
	}
	if !strings.Contains(Box(Welcome{Risky: "Google Drive", Suggest: "x"}, false), "Google Drive copies files to the cloud") {
		t.Error("the welcome box doesn't explain the Google Drive warning")
	}
	updated := WelcomeCard(Welcome{Updated: "x.exe", HasIcon: true}, "1.5.8")
	if updated.Title != "BudMinder is up to date" || !strings.Contains(cardText(updated), "Updated to 1.5.8") {
		t.Errorf("updated card:\n%s", cardText(updated))
	}
	if strings.Contains(cardText(updated), "instead of this file") {
		t.Error("after an update the program file wasn't double-clicked")
	}
	again := WelcomeCard(Welcome{HasIcon: true}, "1.5.8")
	if !strings.Contains(cardText(again), "from the Start menu instead of this file") {
		t.Errorf("started from the file again:\n%s", cardText(again))
	}
	noTray := WelcomeCard(Welcome{LANAddress: "192.168.1.5"}, "1.5.8")
	if strings.Contains(cardText(noTray), "runs in the background") || strings.Contains(cardText(noTray), "on your phone") {
		t.Errorf("without the tray icon there's no tray section or phone link:\n%s", cardText(noTray))
	}
	if !strings.Contains(cardText(noTray), "Stop BudMinder in the Start menu") {
		t.Error("without the tray icon, say how to stop it")
	}
	second := WelcomeCard(Welcome{Elsewhere: `D:\BudMinder`}, "1.5.8")
	if b := findBlock(second, BlockBanner); b == nil || !strings.Contains(spansText(b.Text), "second copy") {
		t.Errorf("second copy banner = %+v", b)
	}
	if setup := WelcomeCard(Welcome{}, "1.5.8"); strings.Contains(cardText(setup), "username and password") {
		t.Error("only a new install is asked to make an account")
	}
}

func TestTrayHint(t *testing.T) {
	t.Parallel()
	if _, text := TrayHint(false); !strings.Contains(text, "including stopping BudMinder") {
		t.Errorf("visible icon hint = %q", text)
	}
	if _, text := TrayHint(true); !strings.Contains(text, "behind the ^") {
		t.Errorf("hidden icon hint = %q", text)
	}
}

func TestProblemCards(t *testing.T) {
	t.Parallel()
	older := OtherRunningCard("", "8080", true)
	if older.Title != "An older BudMinder is still running" || !older.Bad || !strings.Contains(cardText(older), "[Stop it and start this one]") {
		t.Errorf("older card:\n%s", cardText(older))
	}
	if c := OtherRunningCard("1.6.0", "8080", false); c.Title != "Another BudMinder is still running" || !strings.Contains(cardText(c), "BudMinder 1.6.0 is still running from this folder") {
		t.Errorf("newer card:\n%s", cardText(c))
	}
	port := PortTakenCard("8080", "8081", "bind: in use")
	if !strings.Contains(cardText(port), "[Use port 8081]") || findBlock(port, BlockDetails) == nil {
		t.Errorf("port card:\n%s", cardText(port))
	}
	if c := PortTakenCard("8080", "", "x"); strings.Contains(cardText(c), "Use port") {
		t.Error("no free port, no offer")
	}
	if c := StartFailedCard("Failed to open database: unable to open database file (14)"); !strings.Contains(c.Title, "grow journal") {
		t.Errorf("database problem title = %q", c.Title)
	}
	if c := StartFailedCard("Failed to construct HTTP engine: x"); c.Title != "BudMinder couldn't start" {
		t.Errorf("other problem title = %q", c.Title)
	} else if b := findBlock(c, BlockLink); b == nil || b.ID != CardHelp {
		t.Errorf("problem card has no Get help link: %+v", b)
	}
	if c := UpdateCard(`C:\BM`, "1.5.7", "1.5.8"); !strings.Contains(cardText(c), "BudMinder 1.5.7 is installed in C:\\BM. This download is 1.5.8.") {
		t.Errorf("update card:\n%s", cardText(c))
	}
}

func TestSavedPort(t *testing.T) {
	t.Parallel()
	data := filepath.Join(t.TempDir(), "data")
	if p := SavedPort(data); p != "" {
		t.Errorf("no file: %q", p)
	}
	if err := SavePort(data, "8081"); err != nil {
		t.Fatal(err)
	}
	if p := SavedPort(data); p != "8081" {
		t.Errorf("saved port = %q", p)
	}
	_ = os.WriteFile(filepath.Join(data, portFile), []byte("eighty"), 0o644)
	if p := SavedPort(data); p != "" {
		t.Errorf("nonsense in the file: %q", p)
	}
	if p := FreePortAfter("not a port"); p != "" {
		t.Errorf("FreePortAfter(nonsense) = %q", p)
	}
}

func TestRunningHere(t *testing.T) {
	t.Parallel()
	data := t.TempDir()
	if _, ok := RunningHere(data); ok {
		t.Fatal("nothing runs yet")
	}
	ctl, err := StartControl(data, "1.5.8")
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := RunningHere(data); !ok || v != "1.5.8" {
		t.Errorf("RunningHere = %q, %v", v, ok)
	}
	ctl.Close()
	if _, ok := RunningHere(data); ok {
		t.Error("after Close it isn't running")
	}
}

func TestSetDesktopIcon_Off(t *testing.T) {
	t.Parallel()
	if !isDesktopShortcut(`C:\Users\you\Desktop\BudMinder.lnk`) || isDesktopShortcut(`C:\Users\you\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\BudMinder.lnk`) {
		t.Error("isDesktopShortcut mixes up the desktop and the Start menu")
	}
	if err := SetDesktopIcon("x", t.TempDir(), nil, false); err == nil && isWindows() {
		t.Error("no marker: nothing to change")
	}
	if !isWindows() {
		return
	}
	data, desk := t.TempDir(), t.TempDir()
	exe := `C:\BudMinder\budminder-windows-amd64.exe`
	lnk := filepath.Join(desk, "BudMinder.lnk")
	menu := `C:\Users\you\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\BudMinder.lnk`
	_ = os.WriteFile(lnk, []byte("shortcut"), 0o644)
	if err := writeMarker(data, exe, []string{menu, lnk}); err != nil {
		t.Fatal(err)
	}
	if err := SetDesktopIcon(exe, data, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lnk); err == nil {
		t.Error("the desktop icon is still there")
	}
	made, _ := readMarker(data)
	if !reflect.DeepEqual(made, []string{exe, menu, noDesktop}) {
		t.Errorf("marker = %q", made)
	}
}

func isWindows() bool { return runtime.GOOS == "windows" }
