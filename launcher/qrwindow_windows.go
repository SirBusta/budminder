//go:build windows

package launcher

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"rsc.io/qr"
)

// The tray's "Open on your phone…": a small window with a QR code for the
// home-network address, drawn with plain Windows calls (no browser, nothing
// to sign in to; the phone signs in on its own). It looks like the tour: a
// paper background, a tan frame with the seedling in its top edge, and Font
// Awesome icons (the app's own font file, loaded privately). Under the code:
// how to scan, then help for when the phone can't connect (the firewall).

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	pRegisterClassExW             = user32.NewProc("RegisterClassExW")
	pCreateWindowExW              = user32.NewProc("CreateWindowExW")
	pDefWindowProcW               = user32.NewProc("DefWindowProcW")
	pDestroyWindow                = user32.NewProc("DestroyWindow")
	pPostQuitMessage              = user32.NewProc("PostQuitMessage")
	pGetMessageW                  = user32.NewProc("GetMessageW")
	pTranslateMessage             = user32.NewProc("TranslateMessage")
	pDispatchMessageW             = user32.NewProc("DispatchMessageW")
	pBeginPaint                   = user32.NewProc("BeginPaint")
	pEndPaint                     = user32.NewProc("EndPaint")
	pFillRect                     = user32.NewProc("FillRect")
	pDrawTextW                    = user32.NewProc("DrawTextW")
	pShowWindow                   = user32.NewProc("ShowWindow")
	pSetForegroundWindow          = user32.NewProc("SetForegroundWindow")
	pGetSystemMetrics             = user32.NewProc("GetSystemMetrics")
	pSendMessageW                 = user32.NewProc("SendMessageW")
	pLoadCursorW                  = user32.NewProc("LoadCursorW")
	pSetCursor                    = user32.NewProc("SetCursor")
	pGetCursorPos                 = user32.NewProc("GetCursorPos")
	pScreenToClient               = user32.NewProc("ScreenToClient")
	pInvalidateRect               = user32.NewProc("InvalidateRect")
	pEnableWindow                 = user32.NewProc("EnableWindow")
	pAdjustWindowRect             = user32.NewProc("AdjustWindowRect")
	pCreateIconFromResourceEx     = user32.NewProc("CreateIconFromResourceEx")
	pSetThreadDpiAwarenessContext = user32.NewProc("SetThreadDpiAwarenessContext")
	pGetDpiForSystem              = user32.NewProc("GetDpiForSystem")
	pGetDC                        = user32.NewProc("GetDC")
	pReleaseDC                    = user32.NewProc("ReleaseDC")
	pCreateFontW                  = gdi32.NewProc("CreateFontW")
	pSelectObject                 = gdi32.NewProc("SelectObject")
	pDeleteObject                 = gdi32.NewProc("DeleteObject")
	pSetTextColor                 = gdi32.NewProc("SetTextColor")
	pSetBkMode                    = gdi32.NewProc("SetBkMode")
	pGetStockObject               = gdi32.NewProc("GetStockObject")
	pCreateSolidBrush             = gdi32.NewProc("CreateSolidBrush")
	pCreatePen                    = gdi32.NewProc("CreatePen")
	pRoundRect                    = gdi32.NewProc("RoundRect")
	pMoveToEx                     = gdi32.NewProc("MoveToEx")
	pLineTo                       = gdi32.NewProc("LineTo")
	pAddFontMemResourceEx         = gdi32.NewProc("AddFontMemResourceEx")
	pGetModuleHandleW             = kernel32.NewProc("GetModuleHandleW")
)

type wndClassEx struct {
	size               uint32
	style              uint32
	wndProc            uintptr
	clsExtra, wndExtra int32
	instance, icon     windows.Handle
	cursor, background windows.Handle
	menuName           *uint16
	className          *uint16
	iconSm             windows.Handle
}

type rect struct{ left, top, right, bottom int32 }

type point struct{ x, y int32 }

type paintStruct struct {
	hdc                windows.Handle
	erase              int32
	paint              rect
	restore, incUpdate int32
	reserved           [32]byte
}

type msg struct {
	hwnd     windows.Handle
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX, ptY int32
}

type drawItem struct {
	ctlType, ctlID, itemID, itemAction, itemState uint32
	hwndItem, hdc                                 windows.Handle
	item                                          rect
	itemData                                      uintptr
}

const (
	wmDestroy   = 0x0002
	wmPaint     = 0x000F
	wmClose     = 0x0010
	wmSetCursor = 0x0020
	wmDrawItem  = 0x002B
	wmSetFont   = 0x0030
	wmKeyDown   = 0x0100
	wmCommand   = 0x0111
	wmLButtonUp = 0x0202
	wsCaption   = 0x00C00000
	wsSysMenu   = 0x00080000
	wsChild     = 0x40000000
	wsVisible   = 0x10000000
	wsTabStop   = 0x00010000
	bsOwnerDraw = 0x0000000B
	dtCenter    = 0x0001
	dtVCenter   = 0x0004
	dtSingle    = 0x0020
	dtWordBrk   = 0x0010
	dtNoPrefix  = 0x0800
	dtCalcRect  = 0x0400
	odsSelected = 0x0001

	copyButton     = 1
	firewallButton = 2
)

// Colors (Windows wants 0x00BBGGRR), from the tour's palette.
const (
	colPaper  = 0x00F3F9FB // #fbf9f3
	colRule   = 0x0097BFCD // #cdbf97, the frame and divider
	colGreen  = 0x004AA316 // #16a34a, icons and the seedling
	colTitle  = 0x002A4524 // #24452a
	colText   = 0x002B352B // #2b352b
	colTile   = 0x00D0DFE3 // #e3dfd0, around the code
	colLink   = 0x00A86A2F // #2f6aa8
	colButton = 0x00FDFDFD
	colPress  = 0x00DAE9EE // #eee9da
	colBorder = 0x00A4B4B9 // #b9b4a4
	colBad    = 0x001823B4 // #b42318
)

// Font Awesome 6 (solid) icons used here.
const (
	faSeedling = ''
	faCamera   = ''
	faWifi     = ''
	faLock     = ''
	faShield   = ''
	faHand     = ''
	faCopy     = ''
)

// PhoneCode is what the phone window shows.
type PhoneCode struct {
	Address string // "" when there's no home network
	Icon    []byte // 32 px PNG for the title bar (may be nil)
	Glyphs  []byte // Font Awesome solid .ttf (may be nil: no icons)
	Exe     string // this program, for Allow through firewall
}

type phoneWindow struct {
	PhoneCode
	hwnd     windows.Handle
	code     *qr.Code
	dpi      int32
	fonts    struct{ title, addr, head, body, icon, seed windows.Handle }
	buttons  [2]windows.Handle
	link     rect   // "Open Windows firewall settings", for clicks
	status   string // after Allow through firewall
	statusOK bool
	copied   bool
	working  bool // waiting for the elevated copy
}

var (
	phoneMu   sync.Mutex
	phoneOpen *phoneWindow
	wndProcCB = sync.OnceValue(func() uintptr { return windows.NewCallback(phoneWndProc) })
	classOnce sync.Once
	className = windows.StringToUTF16Ptr("BudMinderPhoneCode")
	glyphOnce sync.Once
	glyphData []byte // kept alive: Windows reads the font from this memory
	haveGlyph bool
)

// ShowPhoneCode opens the phone window. A window that's already open comes
// to the front instead.
func ShowPhoneCode(p PhoneCode) {
	phoneMu.Lock()
	if phoneOpen != nil {
		h := phoneOpen.hwnd
		phoneMu.Unlock()
		pSetForegroundWindow.Call(uintptr(h))
		return
	}
	w := &phoneWindow{PhoneCode: p}
	phoneOpen = w
	phoneMu.Unlock()
	if p.Address != "" {
		w.code, _ = qr.Encode(p.Address, qr.M)
	}
	go w.run()
}

// scale turns a size at 100% into this screen's pixels.
func (w *phoneWindow) scale(v int32) int32 { return v * w.dpi / 96 }

func (w *phoneWindow) run() {
	runtime.LockOSThread() // the window's messages arrive on the thread that made it
	defer func() {
		for _, f := range []windows.Handle{w.fonts.title, w.fonts.addr, w.fonts.head, w.fonts.body, w.fonts.icon, w.fonts.seed} {
			pDeleteObject.Call(uintptr(f))
		}
		phoneMu.Lock()
		phoneOpen = nil
		phoneMu.Unlock()
	}()

	// Sharp on scaled screens (125%, 150%): this thread draws at the real size.
	if pSetThreadDpiAwarenessContext.Find() == nil {
		pSetThreadDpiAwarenessContext.Call(^uintptr(1)) // DPI_AWARENESS_CONTEXT_SYSTEM_AWARE (-2)
	}
	w.dpi = 96
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d != 0 {
			w.dpi = int32(d)
		}
	}
	glyphOnce.Do(func() {
		if len(w.Glyphs) == 0 {
			return
		}
		glyphData = w.Glyphs
		var n uint32
		h, _, _ := pAddFontMemResourceEx.Call(uintptr(unsafe.Pointer(&glyphData[0])), uintptr(len(glyphData)), 0, uintptr(unsafe.Pointer(&n)))
		haveGlyph = h != 0
	})
	instance, _, _ := pGetModuleHandleW.Call(0)
	classOnce.Do(func() {
		cursor, _, _ := pLoadCursorW.Call(0, 32512) // IDC_ARROW
		paper, _, _ := pCreateSolidBrush.Call(colPaper)
		wc := wndClassEx{wndProc: wndProcCB(), instance: windows.Handle(instance), cursor: windows.Handle(cursor), background: windows.Handle(paper), className: className}
		wc.size = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	font := func(pt, weight int32, face string) windows.Handle {
		h, _, _ := pCreateFontW.Call(uintptr(-pt*w.dpi/72), 0, 0, 0, uintptr(weight), 0, 0, 0, 1 /* DEFAULT_CHARSET */, 0, 0, 5 /* CLEARTYPE_QUALITY */, 0,
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(face))))
		return windows.Handle(h)
	}
	w.fonts.title = font(15, 700, "Segoe UI")
	w.fonts.addr = font(14, 700, "Segoe UI")
	w.fonts.head = font(11, 700, "Segoe UI")
	w.fonts.body = font(10, 400, "Segoe UI")
	w.fonts.icon = font(11, 900, "Font Awesome 6 Free Solid")
	w.fonts.seed = font(14, 900, "Font Awesome 6 Free Solid")

	// Window size from its content, centered on the main screen.
	width, height := w.layout(0)
	r := rect{0, 0, width, height}
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), wsCaption|wsSysMenu, 0)
	ww, wh := r.right-r.left, r.bottom-r.top
	sw, _, _ := pGetSystemMetrics.Call(0)           // SM_CXSCREEN
	sh, _, _ := pGetSystemMetrics.Call(1)           // SM_CYSCREEN
	hwnd, _, _ := pCreateWindowExW.Call(0x00000008, // WS_EX_TOPMOST: not lost behind the browser
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Open BudMinder on your phone"))),
		wsCaption|wsSysMenu, uintptr((int32(sw)-ww)/2), uintptr(max(0, (int32(sh)-wh)/2)), uintptr(ww), uintptr(wh), 0, 0, instance, 0)
	if hwnd == 0 {
		return
	}
	phoneMu.Lock()
	w.hwnd = windows.Handle(hwnd)
	phoneMu.Unlock()
	if len(w.Icon) > 0 {
		if h, _, _ := pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&w.Icon[0])), uintptr(len(w.Icon)), 1, 0x00030000, 0, 0, 0); h != 0 {
			pSendMessageW.Call(hwnd, 0x0080 /* WM_SETICON */, 0, h)
			pSendMessageW.Call(hwnd, 0x0080, 1, h)
		}
	}
	if w.Address != "" {
		for i, b := range w.buttonRects() {
			h, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("BUTTON"))),
				uintptr(unsafe.Pointer(windows.StringToUTF16Ptr([]string{"Copy address", "Allow through firewall"}[i]))), wsChild|wsVisible|wsTabStop|bsOwnerDraw,
				uintptr(b.left), uintptr(b.top), uintptr(b.right-b.left), uintptr(b.bottom-b.top), hwnd, uintptr(i+1), instance, 0)
			w.buttons[i] = windows.Handle(h)
		}
	}
	pShowWindow.Call(hwnd, 5) // SW_SHOW
	pSetForegroundWindow.Call(hwnd)

	var m msg
	for {
		if r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		if m.message == wmKeyDown && m.wParam == 0x1B { // Esc closes, wherever the focus is
			pSendMessageW.Call(hwnd, wmClose, 0, 0)
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// The words. Each point is its own line.
var (
	phoneSteps = []struct {
		icon rune
		text string
	}{
		{faCamera, "Point your phone's camera at the code, then tap the link that appears."},
		{faWifi, "Your phone must be on the same Wi-Fi as this computer."},
		{faLock, "Sign in on the phone with your BudMinder username and password."},
	}
	phoneHelp = []struct {
		icon rune
		text string
	}{
		{faShield, "Windows may have asked whether to let BudMinder use your network. It needs Allow for private networks."},
		{faHand, "Missed it, or clicked Cancel? Use Allow through firewall below. Windows asks you to confirm."},
	}
)

const phoneNoNetwork = "This computer isn't connected to a home network right now, so a phone can't reach BudMinder. Connect to your Wi-Fi and try again."

// Layout, at 100% (scale makes it real pixels).
const (
	lyWidth   = 390 // client area
	lyOuter   = 16  // paper around the frame
	lyInner   = 22  // frame to content
	lyTop     = 30  // frame's top edge to the title (room for the seedling)
	lyQR      = 200 // the code, about
	lyIconCol = 26  // icon column in front of each line
	lyButtonH = 32
)

func (w *phoneWindow) buttonRects() [2]rect {
	_, _ = w.layout(0) // fills w.link; the buttons sit just above it
	bh := w.scale(lyButtonH)
	widths := [2]int32{w.scale(140), w.scale(190)}
	gap := w.scale(10)
	total := widths[0] + gap + widths[1]
	left := (w.scale(lyWidth) - total) / 2
	top := w.link.top - w.scale(10) - bh
	return [2]rect{{left, top, left + widths[0], top + bh}, {left + widths[0] + gap, top, left + total, top + bh}}
}

// layout places everything and, with hdc, draws it. Returns the client size.
func (w *phoneWindow) layout(hdc windows.Handle) (width, height int32) {
	width = w.scale(lyWidth)
	drawing := hdc != 0
	dc := hdc
	if !drawing { // only measuring: the screen's DC has the same fonts
		d, _, _ := pGetDC.Call(0)
		dc = windows.Handle(d)
		defer pReleaseDC.Call(0, d)
	}
	pSetBkMode.Call(uintptr(dc), 1) // TRANSPARENT
	x0, x1 := w.scale(lyOuter+lyInner), width-w.scale(lyOuter+lyInner)
	frameTop := w.scale(lyOuter + 6)
	y := frameTop + w.scale(lyTop)

	// text draws s in r (word-wrapped; flags add centering), returns its bottom.
	text := func(s string, f windows.Handle, color uintptr, r rect, flags uintptr) int32 {
		p := windows.StringToUTF16Ptr(s)
		old, _, _ := pSelectObject.Call(uintptr(dc), uintptr(f))
		defer pSelectObject.Call(uintptr(dc), old)
		calc := r // measuring narrows it to the text; drawing keeps the width to center in
		pDrawTextW.Call(uintptr(dc), uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&calc)), flags|dtWordBrk|dtNoPrefix|dtCalcRect)
		r.bottom = calc.bottom
		if drawing {
			pSetTextColor.Call(uintptr(dc), color)
			pDrawTextW.Call(uintptr(dc), uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags|dtWordBrk|dtNoPrefix)
		}
		return r.bottom
	}
	icon := func(g rune, f windows.Handle, r rect) {
		if drawing && haveGlyph {
			text(string(g), f, colGreen, r, dtCenter|dtSingle)
		}
	}
	line := func(color uintptr, x, y, toX int32) {
		if !drawing {
			return
		}
		pen, _, _ := pCreatePen.Call(0, uintptr(max(1, w.scale(1))), color)
		old, _, _ := pSelectObject.Call(uintptr(dc), pen)
		pMoveToEx.Call(uintptr(dc), uintptr(x), uintptr(y), 0)
		pLineTo.Call(uintptr(dc), uintptr(toX), uintptr(y))
		pSelectObject.Call(uintptr(dc), old)
		pDeleteObject.Call(pen)
	}
	rows := func(list []struct {
		icon rune
		text string
	}) {
		for _, s := range list {
			icon(s.icon, w.fonts.icon, rect{x0, y + w.scale(1), x0 + w.scale(lyIconCol) - w.scale(6), y + w.scale(20)})
			y = text(s.text, w.fonts.body, colText, rect{x0 + w.scale(lyIconCol), y, x1, y + w.scale(200)}, 0) + w.scale(8)
		}
	}

	y = text("Open BudMinder on your phone", w.fonts.title, colTitle, rect{x0, y, x1, y + w.scale(100)}, dtCenter) + w.scale(14)
	if w.code == nil {
		rows([]struct {
			icon rune
			text string
		}{{faWifi, phoneNoNetwork}})
	} else {
		// The code on a white tile: whole pixels per square, 4-square margin.
		n := int32(w.code.Size) + 8
		cell := max(w.scale(lyQR)/n, 2)
		side := cell * n
		left := (width - side) / 2
		if drawing {
			w.box(dc, rect{left - w.scale(6), y - w.scale(6), left + side + w.scale(6), y + side + w.scale(6)}, 0x00FFFFFF, colTile, w.scale(10))
			black, _, _ := pGetStockObject.Call(4) // BLACK_BRUSH
			for qy := 0; qy < w.code.Size; qy++ {
				for qx := 0; qx < w.code.Size; qx++ {
					if w.code.Black(qx, qy) {
						r := rect{left + (int32(qx)+4)*cell, y + (int32(qy)+4)*cell, left + (int32(qx)+5)*cell, y + (int32(qy)+5)*cell}
						pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&r)), black)
					}
				}
			}
		}
		y += side + w.scale(16)
		y = text(w.Address, w.fonts.addr, 0x00111111, rect{x0, y, x1, y + w.scale(60)}, dtCenter) + w.scale(16)
		rows(phoneSteps)

		// The divider: two lines with the seedling between them.
		y += w.scale(6)
		mid, half := width/2, w.scale(16)
		line(colRule, x0, y+w.scale(8), mid-half)
		line(colRule, mid+half, y+w.scale(8), x1)
		icon(faSeedling, w.fonts.icon, rect{mid - half, y, mid + half, y + w.scale(17)})
		y += w.scale(28)

		y = text("Phone can't connect?", w.fonts.head, colTitle, rect{x0, y, x1, y + w.scale(40)}, 0) + w.scale(8)
		rows(phoneHelp)

		// Buttons (child windows, placed by buttonRects), then the link and the result.
		y += w.scale(8) + w.scale(lyButtonH) + w.scale(10)
		linkText := "Open Windows firewall settings"
		bottom := text(linkText, w.fonts.body, colLink, rect{x0, y, x1, y + w.scale(40)}, dtCenter)
		w.link = rect{x0, y, x1, bottom}
		if drawing { // underline it, as a link
			old, _, _ := pSelectObject.Call(uintptr(dc), uintptr(w.fonts.body))
			calc := rect{0, 0, x1 - x0, 100}
			pDrawTextW.Call(uintptr(dc), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(linkText))), ^uintptr(0), uintptr(unsafe.Pointer(&calc)), dtSingle|dtNoPrefix|dtCalcRect)
			pSelectObject.Call(uintptr(dc), old)
			tw := calc.right
			line(colLink, mid-tw/2, bottom-w.scale(2), mid+tw/2)
		}
		y = bottom + w.scale(8)
		phoneMu.Lock()
		status, ok := w.status, w.statusOK
		phoneMu.Unlock()
		color := uintptr(colGreen)
		if !ok {
			color = colBad
		}
		// Always room for one line of result, so the window needn't grow.
		end := text(" ", w.fonts.body, colText, rect{x0, y, x1, y + w.scale(60)}, dtCenter)
		if status != "" {
			end = max(end, text(status, w.fonts.body, color, rect{x0, y, x1, y + w.scale(60)}, dtCenter))
		}
		y = end
	}
	frameBottom := y + w.scale(lyInner)
	height = frameBottom + w.scale(lyOuter)
	if drawing {
		// The frame, then the seedling sitting in a gap in its top edge.
		w.box(dc, rect{w.scale(lyOuter), frameTop, width - w.scale(lyOuter), frameBottom}, ^uintptr(0), colRule, w.scale(18))
		if haveGlyph {
			gap := rect{width/2 - w.scale(20), frameTop - w.scale(12), width/2 + w.scale(20), frameTop + w.scale(12)}
			paper, _, _ := pCreateSolidBrush.Call(colPaper)
			pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&gap)), paper)
			pDeleteObject.Call(paper)
			icon(faSeedling, w.fonts.seed, gap)
		}
	}
	return width, height
}

// box draws a rounded rectangle; fill ^0 leaves the inside alone.
func (w *phoneWindow) box(dc windows.Handle, r rect, fill, border uintptr, round int32) {
	pen, _, _ := pCreatePen.Call(0, uintptr(max(1, w.scale(3)/2)), border)
	brush, _, _ := pGetStockObject.Call(5) // NULL_BRUSH
	if fill != ^uintptr(0) {
		brush, _, _ = pCreateSolidBrush.Call(fill)
		defer pDeleteObject.Call(brush)
	}
	oldPen, _, _ := pSelectObject.Call(uintptr(dc), pen)
	oldBrush, _, _ := pSelectObject.Call(uintptr(dc), brush)
	pRoundRect.Call(uintptr(dc), uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom), uintptr(round), uintptr(round))
	pSelectObject.Call(uintptr(dc), oldPen)
	pSelectObject.Call(uintptr(dc), oldBrush)
	pDeleteObject.Call(pen)
}

// drawButton paints one of the two buttons (owner-drawn, to match the window).
func (w *phoneWindow) drawButton(d *drawItem) {
	fill := uintptr(colButton)
	if d.itemState&odsSelected != 0 {
		fill = colPress
	}
	paper, _, _ := pCreateSolidBrush.Call(colPaper)
	pFillRect.Call(uintptr(d.hdc), uintptr(unsafe.Pointer(&d.item)), paper)
	pDeleteObject.Call(paper)
	w.box(d.hdc, d.item, fill, colBorder, w.scale(8))
	label, g := "Copy address", faCopy
	phoneMu.Lock()
	switch {
	case d.ctlID == copyButton && w.copied:
		label = "Copied"
	case d.ctlID == firewallButton && w.working:
		label, g = "Waiting for Windows…", faShield
	case d.ctlID == firewallButton:
		label, g = "Allow through firewall", faShield
	}
	phoneMu.Unlock()
	pSetBkMode.Call(uintptr(d.hdc), 1)
	// Icon and words centered together.
	measure := func(s string, f windows.Handle) int32 {
		old, _, _ := pSelectObject.Call(uintptr(d.hdc), uintptr(f))
		defer pSelectObject.Call(uintptr(d.hdc), old)
		r := rect{0, 0, 1000, 100}
		pDrawTextW.Call(uintptr(d.hdc), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), dtSingle|dtNoPrefix|dtCalcRect)
		return r.right
	}
	gw := int32(0)
	if haveGlyph {
		gw = measure(string(g), w.fonts.icon) + w.scale(7)
	}
	tw := measure(label, w.fonts.body)
	x := d.item.left + (d.item.right-d.item.left-gw-tw)/2
	put := func(s string, f windows.Handle, color uintptr, from, to int32) {
		old, _, _ := pSelectObject.Call(uintptr(d.hdc), uintptr(f))
		pSetTextColor.Call(uintptr(d.hdc), color)
		r := rect{from, d.item.top, to, d.item.bottom}
		pDrawTextW.Call(uintptr(d.hdc), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), dtSingle|dtVCenter|dtNoPrefix)
		pSelectObject.Call(uintptr(d.hdc), old)
	}
	if haveGlyph {
		put(string(g), w.fonts.icon, colGreen, x, x+gw)
	}
	put(label, w.fonts.body, 0x001A1A1A, x+gw, x+gw+tw+w.scale(2))
}

// allowFirewall runs the elevated copy, then shows how it went.
func (w *phoneWindow) allowFirewall() {
	phoneMu.Lock()
	if w.working {
		phoneMu.Unlock()
		return
	}
	w.working, w.status = true, ""
	phoneMu.Unlock()
	w.refresh()
	go func() {
		err := AllowFirewallElevated(w.Exe, w.hwnd)
		phoneMu.Lock()
		w.working = false
		switch {
		case err == nil:
			w.status, w.statusOK = "Done. BudMinder is allowed through the firewall on private networks.", true
		case errors.Is(err, ErrCancelled):
			w.status, w.statusOK = "Nothing was changed: Windows' confirmation was cancelled.", false
		default:
			w.status, w.statusOK = "That didn't work. Try Open Windows firewall settings instead.", false
		}
		phoneMu.Unlock()
		w.refresh()
	}()
}

func (w *phoneWindow) refresh() {
	pInvalidateRect.Call(uintptr(w.hwnd), 0, 1)
	for _, b := range w.buttons {
		pInvalidateRect.Call(uintptr(b), 0, 1)
	}
}

func phoneWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	phoneMu.Lock()
	w := phoneOpen
	phoneMu.Unlock()
	switch message {
	case wmPaint:
		if w != nil {
			var ps paintStruct
			hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			w.layout(windows.Handle(hdc))
			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		}
	case wmDrawItem:
		if w != nil {
			w.drawButton(*(**drawItem)(unsafe.Pointer(&lParam))) // lParam points at a DRAWITEMSTRUCT
			return 1
		}
	case wmCommand:
		if w == nil {
			break
		}
		switch wParam & 0xffff {
		case copyButton:
			if copyText(w.Address) == nil {
				phoneMu.Lock()
				w.copied = true
				phoneMu.Unlock()
				w.refresh()
			}
			return 0
		case firewallButton:
			w.allowFirewall()
			return 0
		}
	case wmSetCursor:
		if w != nil && w.overLink() {
			hand, _, _ := pLoadCursorW.Call(0, 32649) // IDC_HAND
			pSetCursor.Call(hand)
			return 1
		}
	case wmLButtonUp:
		if w != nil && w.overLink() {
			_ = OpenFirewallSettings()
			return 0
		}
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

// overLink: the mouse is on "Open Windows firewall settings".
func (w *phoneWindow) overLink() bool {
	if w.code == nil {
		return false
	}
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	pScreenToClient.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&p)))
	return p.x >= w.link.left && p.x < w.link.right && p.y >= w.link.top && p.y < w.link.bottom
}
