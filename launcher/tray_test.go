package launcher

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

// icoImages reads back each PNG in an .ico file made by icoOf.
func icoImages(t *testing.T, ico []byte) []image.Image {
	t.Helper()
	le := binary.LittleEndian
	if le.Uint16(ico[2:]) != 1 {
		t.Fatalf("not an icon: % x", ico[:6])
	}
	var out []image.Image
	for i := 0; i < int(le.Uint16(ico[4:])); i++ {
		e := ico[6+16*i:]
		size, off := le.Uint32(e[8:]), le.Uint32(e[12:])
		img, err := png.Decode(bytes.NewReader(ico[off : off+size]))
		if err != nil {
			t.Fatalf("image %d: %v", i, err)
		}
		if w := img.Bounds().Dx(); w != int(e[0]) {
			t.Errorf("image %d is %d wide, directory says %d", i, w, e[0])
		}
		out = append(out, img)
	}
	return out
}

func TestTrayIcon_SizesAndBorder(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("../web/static/img/brand/budminder-256.png")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := TrayIcon(src, false)
	if err != nil {
		t.Fatal(err)
	}
	framed, err := TrayIcon(src, true)
	if err != nil {
		t.Fatal(err)
	}
	p, f := icoImages(t, plain), icoImages(t, framed)
	if len(p) != len(trayIconSizes) || len(f) != len(trayIconSizes) {
		t.Fatalf("want %d sizes, got %d and %d", len(trayIconSizes), len(p), len(f))
	}
	for i, n := range trayIconSizes {
		if p[i].Bounds().Dx() != n {
			t.Errorf("size %d: got %d", n, p[i].Bounds().Dx())
		}
		// One pixel in from the left edge, halfway down: amber in the frame,
		// the icon's green without it.
		c := color.NRGBAModel.Convert(f[i].At(1, n/2)).(color.NRGBA)
		if c.R < 0xd0 || c.G < 0x80 || c.G > 0xb0 || c.B > 0x40 {
			t.Errorf("size %d: frame pixel is %02x%02x%02x, want amber", n, c.R, c.G, c.B)
		}
		if c := color.NRGBAModel.Convert(p[i].At(1, n/2)).(color.NRGBA); c.R > 0x80 {
			t.Errorf("size %d: the plain icon's edge is %02x%02x%02x, want green", n, c.R, c.G, c.B)
		}
	}
}

func TestIconPNG(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("../web/static/img/brand/budminder-256.png")
	if err != nil {
		t.Fatal(err)
	}
	b, err := IconPNG(src, 32)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil || img.Bounds().Dx() != 32 {
		t.Fatalf("want a 32 px PNG, got %v %v", img, err)
	}
}

func TestTrayIcon_BadPNG(t *testing.T) {
	t.Parallel()
	if _, err := TrayIcon([]byte("not a png"), false); err == nil {
		t.Error("want an error")
	}
}

func TestTray_Addresses(t *testing.T) {
	t.Parallel()
	tr := Tray{Version: "1.6.0", Port: "8091"}
	if got := tr.URL(); got != "http://localhost:8091" {
		t.Errorf("URL = %q", got)
	}
	if got := tr.UpdateURL(); got != "http://localhost:8091/?update=1" {
		t.Errorf("UpdateURL = %q", got)
	}
	if got := tr.Tooltip(""); got != "BudMinder 1.6.0 is running" {
		t.Errorf("Tooltip = %q", got)
	}
	if got := tr.Tooltip("1.7.0"); !strings.HasSuffix(got, "\nUpdate available: 1.7.0") {
		t.Errorf("Tooltip with update = %q", got)
	}
}

func TestBox_TrayHint(t *testing.T) {
	t.Parallel()
	w := Welcome{Port: "8080", OS: "windows", FromConsole: true}
	if strings.Contains(Box(w, false), "by the clock") {
		t.Error("no tray icon, no tray hint")
	}
	w.Tray = true
	box := Box(w, false)
	for _, want := range []string{"BudMinder's icon is by the clock", "Stop BudMinder", "^ arrow"} {
		if !strings.Contains(box, want) {
			t.Errorf("box lacks %q:\n%s", want, box)
		}
	}
}

func TestFirewallCommands(t *testing.T) {
	t.Parallel()
	got := firewallCommands(`C:\Users\Pat Smith\BudMinder\budminder.exe`)
	if len(got) != 2 {
		t.Fatalf("want delete then add, got %q", got)
	}
	// Old rules for this program go first (a Cancel on Windows' prompt leaves a block rule).
	if got[0] != `netsh advfirewall firewall delete rule name=all program="C:\Users\Pat Smith\BudMinder\budminder.exe"` {
		t.Errorf("delete = %s", got[0])
	}
	for _, want := range []string{`add rule name="BudMinder"`, "dir=in", "action=allow", `program="C:\Users\Pat Smith\BudMinder\budminder.exe"`, "profile=private"} {
		if !strings.Contains(got[1], want) {
			t.Errorf("add lacks %s: %s", want, got[1])
		}
	}
	if strings.Contains(got[1], "public") {
		t.Error("public networks (cafés, airports) stay closed")
	}
}
