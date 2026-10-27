//go:build windows

package launcher

// +parallel:serial — the cards share the test hooks cardOffscreen and cardShown.

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestCards_Draw draws every card off the screen. With BUDMINDER_CARD_SHOTS
// set to a folder, it also saves each as a PNG there, to look at.
func TestCards_Draw(t *testing.T) {
	icon, _ := os.ReadFile(filepath.Join("..", "web", "static", "img", "brand", "budminder-256.png"))
	glyphs, _ := os.ReadFile(filepath.Join("..", "web", "static", "vendor", "fontawesome", "webfonts", "fa-solid-900.ttf"))
	SetLook(icon, glyphs)
	shots := os.Getenv("BUDMINDER_CARD_SHOTS")
	home := `C:\Users\you\BudMinder`
	cases := map[string]Card{
		"welcome-first":   WelcomeCard(Welcome{Port: "8080", Folder: home, Setup: true, IconsMade: true, IconDesktop: true, Tray: true, LANAddress: "192.168.1.5"}, "1.5.8"),
		"welcome-moved":   WelcomeCard(Welcome{Port: "8080", Folder: home, Setup: true, IconsMade: true, IconDesktop: true, Tray: true, Installed: "x"}, "1.5.8"),
		"welcome-stayed":  WelcomeCard(Welcome{Port: "8080", Folder: `C:\Users\you\Downloads`, Setup: true, Risky: "Downloads", Suggest: home, Tray: true}, "1.5.8"),
		"welcome-again":   WelcomeCard(Welcome{Port: "8080", Folder: home, HasIcon: true, Tray: true}, "1.5.8"),
		"welcome-updated": WelcomeCard(Welcome{Port: "8080", Folder: home, Updated: "x", HasIcon: true, Tray: true}, "1.5.8"),
		"update":          UpdateCard(home, "1.5.7", "1.5.8"),
		"older":           OtherRunningCard(home, "", "8080", true),
		"port":            PortTakenCard("8080", "8081", "listen tcp :8080: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted."),
		"database":        StartFailedCard(home, "Failed to create SQLite driver: unable to open database file (14)"),
		"setup":           SetupFailedCard(`D:\Grow\BudMinder`, `open D:\Grow\BudMinder\budminder-windows-amd64.exe: Access is denied.`),
	}
	cardOffscreen = true
	defer func() { cardOffscreen, cardShown = false, nil }()
	for name, c := range cases {
		var size image.Point
		cardShown = func(cw *CardWindow) {
			time.Sleep(400 * time.Millisecond)
			if r, ok := windowRect(cw.hwnd); ok {
				size = image.Pt(int(r.right-r.left), int(r.bottom-r.top))
				if shots != "" {
					if img := capture(cw, size); img != nil {
						f, err := os.Create(filepath.Join(shots, name+".png"))
						if err == nil {
							_ = png.Encode(f, img)
							f.Close()
						}
					}
				}
			}
			cw.Close(CardClose)
		}
		if got := RunCard(c, CardEvents{}); got != CardClose {
			t.Errorf("%s: closed with %d", name, got)
		}
		if size.X < 300 || size.Y < 150 {
			t.Errorf("%s: window %v is too small", name, size)
		}
	}
}

// capture draws the card's inside into an image, the way it paints itself:
// Windows doesn't paint windows that are off the screen.
func capture(cw *CardWindow, _ image.Point) image.Image {
	w, h := cw.layout(0)
	size := image.Pt(int(w), int(h))
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	defer pDeleteDC.Call(mem)
	bi := struct {
		size                 uint32
		width, height        int32
		planes, bitCount     uint16
		compression, sizeImg uint32
		xppm, yppm           int32
		clrUsed, clrImp      uint32
	}{size: 40, width: int32(size.X), height: -int32(size.Y), planes: 1, bitCount: 32}
	var bits unsafe.Pointer
	dib, _, _ := pCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 {
		return nil
	}
	defer pDeleteObject.Call(dib)
	old, _, _ := pSelectObject.Call(mem, dib)
	defer pSelectObject.Call(mem, old)
	paper, _, _ := pCreateSolidBrush.Call(colPaper)
	all := rect{0, 0, w, h}
	pFillRect.Call(mem, uintptr(unsafe.Pointer(&all)), paper)
	pDeleteObject.Call(paper)
	cw.layout(windows.Handle(mem))
	for _, p := range cw.places {
		cw.drawKid(&drawItem{ctlID: uint32(p.id), hdc: windows.Handle(mem), item: p.r})
	}
	px := unsafe.Slice((*byte)(bits), size.X*size.Y*4)
	img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	for i := 0; i < size.X*size.Y; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = px[i*4+2], px[i*4+1], px[i*4], 255
	}
	return img
}
