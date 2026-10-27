//go:build windows

package launcher

import (
	"math"
	"os"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Closing the welcome window doesn't stop BudMinder: the window shrinks into
// the leaf icon by the clock (or into the ^ when Windows hides the icon), the
// spot pulses twice, and the first time a Windows notification says where
// BudMinder went. The icon belongs to fyne.io/systray: its hidden window has
// the class "SystrayClass" and the icon the id 100.

var (
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	pShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
	pShellNotifyIconGetRect = shell32.NewProc("Shell_NotifyIconGetRect")
	pEnumWindows            = user32.NewProc("EnumWindows")
	pGetClassNameW          = user32.NewProc("GetClassNameW")
	pGetWindowThreadProcID  = user32.NewProc("GetWindowThreadProcessId")
	pFindWindowW            = user32.NewProc("FindWindowW")
	pFindWindowExW          = user32.NewProc("FindWindowExW")
	pGetWindowRect          = user32.NewProc("GetWindowRect")
	pIsWindowVisible        = user32.NewProc("IsWindowVisible")
	pPrintWindow            = user32.NewProc("PrintWindow")
	pUpdateLayeredWindow    = user32.NewProc("UpdateLayeredWindow")
	pPeekMessageW           = user32.NewProc("PeekMessageW")
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pStretchBlt             = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")
	pDwmGetWindowAttribute  = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
)

const trayIconID = 100 // fyne.io/systray's one icon

// trayWindow is the tray icon's hidden window in this program, or 0.
func trayWindow() windows.Handle {
	pid := uint32(os.Getpid())
	var found windows.Handle
	cb := windows.NewCallback(func(h, _ uintptr) uintptr {
		var p uint32
		pGetWindowThreadProcID.Call(h, uintptr(unsafe.Pointer(&p)))
		if p != pid {
			return 1
		}
		buf := make([]uint16, 64)
		n, _, _ := pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), 64)
		if windows.UTF16ToString(buf[:n]) == "SystrayClass" {
			found = windows.Handle(h)
			return 0
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	return found
}

func windowRect(h windows.Handle) (rect, bool) {
	var r rect
	ok, _, _ := pGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r)))
	return r, ok != 0 && r.right > r.left
}

// trayTarget is where the icon is on the screen: the icon itself when it
// sits on the taskbar, else the ^ that hides it (hidden true).
func trayTarget() (r rect, hidden, ok bool) {
	bar, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Shell_TrayWnd"))), 0)
	barRect, barOK := windowRect(windows.Handle(bar))
	if tw := trayWindow(); tw != 0 {
		id := struct {
			size uint32
			hwnd windows.Handle
			id   uint32
			guid windows.GUID
		}{hwnd: tw, id: trayIconID}
		id.size = uint32(unsafe.Sizeof(id))
		var ir rect
		if hr, _, _ := pShellNotifyIconGetRect.Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&ir))); hr == 0 && ir.right > ir.left {
			// On the taskbar, or in the hidden-icons box (off it, or not shown).
			if !barOK || (ir.left >= barRect.left && ir.right <= barRect.right && ir.top >= barRect.top && ir.bottom <= barRect.bottom) {
				return ir, false, true
			}
		}
	}
	if !barOK {
		return rect{}, true, false
	}
	// The ^: a Button in the notification area (Windows 10). Windows 11 draws
	// it differently, so there it's the start of the notification area.
	notify, _, _ := pFindWindowExW.Call(bar, 0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("TrayNotifyWnd"))), 0)
	if notify != 0 {
		if chev, _, _ := pFindWindowExW.Call(notify, 0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Button"))), 0); chev != 0 {
			if v, _, _ := pIsWindowVisible.Call(chev); v != 0 {
				if cr, ok := windowRect(windows.Handle(chev)); ok {
					return cr, true, true
				}
			}
		}
		if nr, ok := windowRect(windows.Handle(notify)); ok {
			h := nr.bottom - nr.top
			return rect{nr.left, nr.top, nr.left + min(h, nr.right-nr.left), nr.bottom}, true, true
		}
	}
	return rect{barRect.right - 120, barRect.top, barRect.right - 80, barRect.bottom}, true, true
}

// SendToTray closes the welcome window into the tray icon and, when notify
// is true, says where BudMinder went in a Windows notification. Call it from
// the card's Close event.
func SendToTray(cw *CardWindow, notify bool) {
	target, hidden, ok := trayTarget()
	if ok {
		shrink(cw.hwnd, target)
	}
	cw.result = CardGotIt
	pDestroyWindow.Call(uintptr(cw.hwnd))
	if ok {
		go pulse(target, cw.dpi)
	}
	if notify {
		title, text := TrayHint(hidden)
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = trayNotify(title, text)
		}()
	}
}

// newDIB makes a w×h 32-bit top-down bitmap selected into a new memory DC.
// done puts things back and frees both.
func newDIB(screen uintptr, w, h int32) (mem uintptr, bits unsafe.Pointer, done func()) {
	mem, _, _ = pCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return 0, nil, nil
	}
	bi := struct {
		size                 uint32
		width, height        int32
		planes, bitCount     uint16
		compression, sizeImg uint32
		xppm, yppm           int32
		clrUsed, clrImp      uint32
	}{width: w, height: -h, planes: 1, bitCount: 32}
	bi.size = 40
	dib, _, _ := pCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 {
		pDeleteDC.Call(mem)
		return 0, nil, nil
	}
	old, _, _ := pSelectObject.Call(mem, dib)
	return mem, bits, func() {
		pSelectObject.Call(mem, old)
		pDeleteObject.Call(dib)
		pDeleteDC.Call(mem)
	}
}

// shrink hides the window and flies a picture of it into target, getting
// smaller and fainter (Windows' own DrawAnimatedRects draws nothing while
// DWM is on, so this does it with a layered window).
func shrink(hwnd windows.Handle, target rect) {
	win, ok := windowRect(hwnd)
	if !ok {
		return
	}
	// The part you see: Windows 10 counts invisible resize borders in the
	// window rect.
	from := win
	pDwmGetWindowAttribute.Call(uintptr(hwnd), 9 /* DWMWA_EXTENDED_FRAME_BOUNDS */, uintptr(unsafe.Pointer(&from)), unsafe.Sizeof(from))
	if from.right <= from.left || from.left < win.left || from.right > win.right {
		from = win
	}
	ww, wh := win.right-win.left, win.bottom-win.top
	w, h := from.right-from.left, from.bottom-from.top

	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	snap, _, freeSnap := newDIB(screen, ww, wh)
	if snap == 0 {
		return
	}
	defer freeSnap()
	if r, _, _ := pPrintWindow.Call(uintptr(hwnd), snap, 2 /* PW_RENDERFULLCONTENT */); r == 0 {
		return
	}
	frame, _, freeFrame := newDIB(screen, w, h)
	if frame == 0 {
		return
	}
	defer freeFrame()
	pSetStretchBltMode.Call(frame, 4 /* HALFTONE */)

	instance, _, _ := pGetModuleHandleW.Call(0)
	const exStyle = 0x00080000 | 0x00000020 | 0x00000008 | 0x00000080 | 0x08000000 // LAYERED|TRANSPARENT|TOPMOST|TOOLWINDOW|NOACTIVATE
	fly, _, _ := pCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))), 0,
		0x80000000 /* WS_POPUP */, uintptr(from.left), uintptr(from.top), uintptr(w), uintptr(h), 0, 0, instance, 0)
	if fly == 0 {
		return
	}
	defer pDestroyWindow.Call(fly)

	show := func(r rect, alpha uint32) {
		cw, ch := max(r.right-r.left, 1), max(r.bottom-r.top, 1)
		pStretchBlt.Call(frame, 0, 0, uintptr(cw), uintptr(ch), snap, uintptr(from.left-win.left), uintptr(from.top-win.top), uintptr(w), uintptr(h), 0x00CC0020 /* SRCCOPY */)
		pt := point{r.left, r.top}
		sz := struct{ cx, cy int32 }{cw, ch}
		src := point{}
		blend := alpha << 16 // AC_SRC_OVER, constant alpha, no per-pixel alpha
		pUpdateLayeredWindow.Call(fly, screen, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&sz)), frame, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), 2 /* ULW_ALPHA */)
	}
	show(from, 255)
	pShowWindow.Call(fly, 4)           // SW_SHOWNOACTIVATE, on top of the real one
	pShowWindow.Call(uintptr(hwnd), 0) // SW_HIDE

	// The end size keeps the window's shape, about as tall as the icon.
	th := max(target.bottom-target.top, 8)
	tw := max(th*w/max(h, 1), 8)
	tx, ty := (target.left+target.right)/2, (target.top+target.bottom)/2
	end := rect{tx - tw/2, ty - th/2, tx + tw/2, ty + th/2}
	lerp := func(a, b int32, t float64) int32 { return a + int32(math.Round(float64(b-a)*t)) }

	const steps = 18
	start := time.Now()
	for f := 1; f <= steps; f++ {
		t := float64(f) / steps
		e := t * t // speeds up toward the icon
		show(rect{lerp(from.left, end.left, e), lerp(from.top, end.top, e), lerp(from.right, end.right, e), lerp(from.bottom, end.bottom, e)},
			uint32(255-200*t))
		if wait := time.Duration(f)*16*time.Millisecond - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}
	}
}

// trayNotify shows a Windows notification from the tray icon.
func trayNotify(title, text string) error {
	tw := trayWindow()
	if tw == 0 {
		return windows.ERROR_NOT_FOUND
	}
	nid := struct {
		size                   uint32
		wnd                    windows.Handle
		id, flags, callbackMsg uint32
		icon                   windows.Handle
		tip                    [128]uint16
		state, stateMask       uint32
		info                   [256]uint16
		timeoutOrVersion       uint32
		infoTitle              [64]uint16
		infoFlags              uint32
		guidItem               windows.GUID
		balloonIcon            windows.Handle
	}{wnd: tw, id: trayIconID, flags: 0x10 /* NIF_INFO */}
	nid.size = uint32(unsafe.Sizeof(nid))
	copy(nid.infoTitle[:63], windows.StringToUTF16(title))
	copy(nid.info[:255], windows.StringToUTF16(text))
	nid.infoFlags = 0x80 // NIIF_RESPECT_QUIET_TIME
	if len(look.icon) > 0 {
		if h, _, _ := pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&look.icon[0])), uintptr(len(look.icon)), 1, 0x00030000, 32, 32, 0); h != 0 {
			nid.balloonIcon = windows.Handle(h)
			nid.infoFlags |= 0x04 | 0x20 // NIIF_USER | NIIF_LARGE_ICON
			defer pDestroyIcon.Call(h)
		}
	}
	if r, _, err := pShellNotifyIconW.Call(1 /* NIM_MODIFY */, uintptr(unsafe.Pointer(&nid))); r == 0 {
		return err
	}
	return nil
}

// pulse draws two green rings growing out of r, on top of everything and
// letting clicks through.
func pulse(r rect, dpi int32) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	size := 72 * dpi / 96
	cx, cy := (r.left+r.right)/2, (r.top+r.bottom)/2
	x, y := cx-size/2, cy-size/2
	instance, _, _ := pGetModuleHandleW.Call(0)
	const exStyle = 0x00080000 | 0x00000020 | 0x00000008 | 0x00000080 | 0x08000000 // LAYERED|TRANSPARENT|TOPMOST|TOOLWINDOW|NOACTIVATE
	hwnd, _, _ := pCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))), 0,
		0x80000000 /* WS_POPUP */, uintptr(x), uintptr(y), uintptr(size), uintptr(size), 0, 0, instance, 0)
	if hwnd == 0 {
		return
	}
	defer pDestroyWindow.Call(hwnd)

	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	mem, bits, free := newDIB(screen, size, size)
	if mem == 0 {
		return
	}
	defer free()
	px := unsafe.Slice((*uint32)(bits), int(size*size))
	pShowWindow.Call(hwnd, 4) // SW_SHOWNOACTIVATE

	thick := float64(3 * dpi / 96)
	half := float64(size) / 2
	frames := 40
	var m msg
	for ring := 0; ring < 2; ring++ {
		for f := 0; f <= frames; f++ {
			t := float64(f) / float64(frames)
			rad := half * (0.25 + 0.7*t)
			alpha := 1 - t
			for j := int32(0); j < size; j++ {
				for i := int32(0); i < size; i++ {
					dx, dy := float64(i)-half+0.5, float64(j)-half+0.5
					d := dx*dx + dy*dy
					a := 0.0
					if d > 0 {
						dist := math.Sqrt(d) - rad
						if dist < 0 {
							dist = -dist
						}
						if dist < thick/2+1 {
							a = alpha * min(1, thick/2+1-dist)
						}
					}
					// #4ade80, premultiplied BGRA.
					A := uint32(a * 255)
					px[j*size+i] = A<<24 | (A*0x4a/255)<<16 | (A*0xde/255)<<8 | (A * 0x80 / 255)
				}
			}
			pt := point{x, y}
			sz := struct{ cx, cy int32 }{size, size}
			src := point{}
			blend := uint32(0x01FF0000) // AC_SRC_OVER, 0, 255, AC_SRC_ALPHA
			pUpdateLayeredWindow.Call(hwnd, screen, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&sz)), mem, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), 2 /* ULW_ALPHA */)
			for {
				if r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1); r == 0 {
					break
				}
				pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
				pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
}
