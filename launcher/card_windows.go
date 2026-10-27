//go:build windows

package launcher

import (
	"os/exec"
	"regexp"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Drawing cards (card.go) with plain Windows calls, like the phone window:
// one window per card on its own thread, buttons and the check box as
// owner-drawn child windows, everything else painted.

var (
	pGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	pTextOutW              = gdi32.NewProc("TextOutW")
	pDrawIconEx            = user32.NewProc("DrawIconEx")
	pDestroyIcon           = user32.NewProc("DestroyIcon")
	pSetWindowPos          = user32.NewProc("SetWindowPos")
	pPostMessageW          = user32.NewProc("PostMessageW")
	pSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
)

const (
	wmSysCommand = 0x0112
	wmApp        = 0x8000
	wmCardClose  = wmApp + 1 // wParam: the result
	scMinimize   = 0xF020
	wsMinimize   = 0x00020000 // WS_MINIMIZEBOX
)

// More colors (0x00BBGGRR).
const (
	colOKBack   = 0x00EEF6EA // #eaf6ee
	colOKText   = 0x002C4D1F // #1f4d2c
	colWarnBack = 0x00E1F3FD // #fdf3e1
	colWarnText = 0x00073A6B // #6b3a07
	colWarnIcon = 0x000953B4 // #b45309
	colCodeBack = 0x00E0ECF0 // #f0ece0
	colDetails  = 0x00E6F0F3 // #f3f0e6
	colGrey     = 0x00555555
	colWhite    = 0x00FFFFFF
	colDark     = 0x001A1A1A
)

// Look is the BudMinder icon (256 px PNG) and the Font Awesome font the
// cards draw with; main sets it before the first card.
var look struct{ icon, glyphs []byte }

// SetLook gives the cards their icon and icon font.
func SetLook(icon256, glyphs []byte) { look.icon, look.glyphs = icon256, glyphs }

// CardEvents is what a card's buttons, check box, link and close do. They
// run on the card's own thread: quick work only, start a goroutine for more.
type CardEvents struct {
	// Button: a button or the link. True closes the card with id as the result.
	Button func(cw *CardWindow, id int) bool
	// Check: the check box changed.
	Check func(cw *CardWindow, id int, on bool)
	// Close: the ✕, minimize or Esc. nil closes the card with its last
	// button's id (Not now, Cancel, Close).
	Close func(cw *CardWindow)
}

// CardWindow is a card on the screen.
type CardWindow struct {
	card       Card
	ev         CardEvents
	hwnd       windows.Handle
	dpi        int32
	fonts      struct{ title, body, bold, code, head, small, icon, seed windows.Handle }
	app        windows.Handle // the BudMinder icon, row size
	kids       map[int]windows.Handle
	places     []placed
	links      []placed
	check      map[int]bool
	label      map[int]string
	result     int
	topmostFor time.Duration
}

type placed struct {
	id   int
	r    rect
	main bool
	icon rune
	kind BlockKind
}

// For tests: cards off the screen, and a look at each once it's shown.
var (
	cardOffscreen bool
	cardShown     func(*CardWindow)
)

var (
	cardMu        sync.Mutex
	cards         = map[windows.Handle]*CardWindow{}
	cardClass     = windows.StringToUTF16Ptr("BudMinderCard")
	cardClassOnce sync.Once
	cardProcCB    = sync.OnceValue(func() uintptr { return windows.NewCallback(cardWndProc) })
)

// RunCard shows the card and waits until it closes. It returns the id of
// what closed it (0 if it couldn't be shown).
func RunCard(c Card, ev CardEvents) int {
	return runCard(c, ev, 0)
}

// RunWelcome is RunCard for the welcome window: it starts above the browser
// that opens at the same time, then lets the browser come first.
func RunWelcome(c Card, ev CardEvents) int {
	return runCard(c, ev, 6*time.Second)
}

func runCard(c Card, ev CardEvents, topmostFor time.Duration) int {
	done := make(chan int, 1)
	go func() {
		runtime.LockOSThread() // a window's messages arrive on the thread that made it
		cw := &CardWindow{card: c, ev: ev, kids: map[int]windows.Handle{}, check: map[int]bool{}, label: map[int]string{}, topmostFor: topmostFor}
		done <- cw.run()
	}()
	return <-done
}

// SetLabel changes a button's words (Copy details → Copied).
func (cw *CardWindow) SetLabel(id int, s string) {
	cardMu.Lock()
	cw.label[id] = s
	h := cw.kids[id]
	cardMu.Unlock()
	pInvalidateRect.Call(uintptr(h), 0, 1)
}

// Close closes the card with result. Safe from any goroutine.
func (cw *CardWindow) Close(result int) {
	pPostMessageW.Call(uintptr(cw.hwnd), wmCardClose, uintptr(result), 0)
}

// Hwnd is the card's window.
func (cw *CardWindow) Hwnd() windows.Handle { return cw.hwnd }

func (cw *CardWindow) scale(v int32) int32 { return v * cw.dpi / 96 }

// dismissID is what ✕ means when nothing else is said: the last button.
func (cw *CardWindow) dismissID() int {
	id := CardClose
	for _, b := range cw.card.Blocks {
		if b.Kind == BlockButtons && len(b.Buttons) > 0 {
			id = b.Buttons[len(b.Buttons)-1].ID
		}
	}
	return id
}

func (cw *CardWindow) run() int {
	if pSetThreadDpiAwarenessContext.Find() == nil {
		pSetThreadDpiAwarenessContext.Call(^uintptr(1)) // DPI_AWARENESS_CONTEXT_SYSTEM_AWARE
	}
	cw.dpi = 96
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d != 0 {
			cw.dpi = int32(d)
		}
	}
	loadGlyphs(look.glyphs)
	instance, _, _ := pGetModuleHandleW.Call(0)
	cardClassOnce.Do(func() {
		cursor, _, _ := pLoadCursorW.Call(0, 32512) // IDC_ARROW
		paper, _, _ := pCreateSolidBrush.Call(colPaper)
		wc := wndClassEx{wndProc: cardProcCB(), instance: windows.Handle(instance), cursor: windows.Handle(cursor), background: windows.Handle(paper), className: cardClass}
		wc.size = uint32(unsafe.Sizeof(wc))
		pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	font := func(pt10, weight int32, face string) windows.Handle { // pt10: tenths of a point
		h, _, _ := pCreateFontW.Call(uintptr(-pt10*cw.dpi/720), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(face))))
		return windows.Handle(h)
	}
	cw.fonts.title = font(150, 700, "Segoe UI")
	cw.fonts.body = font(100, 400, "Segoe UI")
	cw.fonts.bold = font(100, 700, "Segoe UI")
	cw.fonts.code = font(95, 400, "Consolas")
	cw.fonts.head = font(110, 700, "Segoe UI")
	cw.fonts.small = font(85, 400, "Consolas")
	cw.fonts.icon = font(105, 900, "Font Awesome 6 Free Solid")
	cw.fonts.seed = font(140, 900, "Font Awesome 6 Free Solid")
	defer func() {
		for _, f := range []windows.Handle{cw.fonts.title, cw.fonts.body, cw.fonts.bold, cw.fonts.code, cw.fonts.head, cw.fonts.small, cw.fonts.icon, cw.fonts.seed} {
			pDeleteObject.Call(uintptr(f))
		}
		if cw.app != 0 {
			pDestroyIcon.Call(uintptr(cw.app))
		}
	}()
	if len(look.icon) > 0 {
		n := cw.scale(16)
		if h, _, _ := pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&look.icon[0])), uintptr(len(look.icon)), 1, 0x00030000, uintptr(n), uintptr(n), 0); h != 0 {
			cw.app = windows.Handle(h)
		}
	}

	width, height := cw.layout(0)
	r := rect{0, 0, width, height}
	style := uintptr(wsCaption | wsSysMenu | wsMinimize)
	pAdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), style, 0)
	ww, wh := r.right-r.left, r.bottom-r.top
	var work rect
	pSystemParametersInfoW.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0) // SPI_GETWORKAREA
	x := work.left + (work.right-work.left-ww)/2
	y := max(work.top, work.top+(work.bottom-work.top-wh)/2)
	if cardOffscreen { // tests: drawn, but not on anyone's screen
		x, y = -4000, -4000
	}
	hwnd, _, _ := pCreateWindowExW.Call(0x00000008, // WS_EX_TOPMOST, for now
		uintptr(unsafe.Pointer(cardClass)), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(cw.card.Caption))),
		style, uintptr(x), uintptr(y), uintptr(ww), uintptr(wh), 0, 0, instance, 0)
	cardMu.Lock()
	if hwnd != 0 {
		cw.hwnd = windows.Handle(hwnd)
		cards[cw.hwnd] = cw
	}
	cardMu.Unlock()
	if hwnd == 0 {
		return 0
	}
	if len(look.icon) > 0 {
		if small, err := IconPNG(look.icon, 32); err == nil && len(small) > 0 {
			if h, _, _ := pCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&small[0])), uintptr(len(small)), 1, 0x00030000, 0, 0, 0); h != 0 {
				pSendMessageW.Call(hwnd, 0x0080, 0, h) // WM_SETICON small
				pSendMessageW.Call(hwnd, 0x0080, 1, h) // and big
			}
		}
	}
	for _, p := range cw.places {
		label := ""
		h, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("BUTTON"))),
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(label))), wsChild|wsVisible|wsTabStop|bsOwnerDraw,
			uintptr(p.r.left), uintptr(p.r.top), uintptr(p.r.right-p.r.left), uintptr(p.r.bottom-p.r.top), hwnd, uintptr(p.id), instance, 0)
		cardMu.Lock()
		cw.kids[p.id] = windows.Handle(h)
		cardMu.Unlock()
	}
	pShowWindow.Call(hwnd, 5) // SW_SHOW
	pSetForegroundWindow.Call(hwnd)
	if cardShown != nil {
		go cardShown(cw)
	}
	if cw.topmostFor > 0 {
		go func() {
			time.Sleep(cw.topmostFor)
			// HWND_NOTOPMOST, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE
			pSetWindowPos.Call(hwnd, ^uintptr(1), 0, 0, 0, 0, 0x0002|0x0001|0x0010)
		}()
	}

	var m msg
	for {
		if r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			break
		}
		if m.message == wmKeyDown && m.wParam == 0x1B { // Esc, wherever the focus is
			pSendMessageW.Call(hwnd, wmClose, 0, 0)
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	cardMu.Lock()
	delete(cards, cw.hwnd)
	res := cw.result
	cardMu.Unlock()
	return res
}

// Layout, at 100%.
const (
	cdWidth   = 460
	cdOuter   = 16
	cdInner   = 22
	cdTop     = 30 // frame's top edge to the first line (room for the seedling)
	cdIconCol = 26
	cdButtonH = 32
	cdMainH   = 38
)

var tokenRE = regexp.MustCompile(`\S+\s*|\s+`)

type token struct {
	s     string
	style SpanStyle
	w     int32
}

// flow lays out (and with dc, draws) spans between x0 and x1 from y. It
// returns the bottom.
func (cw *CardWindow) flow(dc windows.Handle, spans []Span, plain, bold, code windows.Handle, x0, x1, y int32, center bool, color uintptr) int32 {
	measureDC := dc
	if dc == 0 {
		d, _, _ := pGetDC.Call(0)
		measureDC = windows.Handle(d)
		defer pReleaseDC.Call(0, d)
	}
	fontOf := func(s SpanStyle) windows.Handle {
		switch s {
		case Bold:
			return bold
		case Code:
			return code
		}
		return plain
	}
	measure := func(s string, f windows.Handle) (int32, int32) {
		if s == "" {
			return 0, 0
		}
		old, _, _ := pSelectObject.Call(uintptr(measureDC), uintptr(f))
		defer pSelectObject.Call(uintptr(measureDC), old)
		u := windows.StringToUTF16(s)
		var sz struct{ cx, cy int32 }
		pGetTextExtentPoint32W.Call(uintptr(measureDC), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
		return sz.cx, sz.cy
	}
	_, lineH := measure("Ag", plain)
	lineH += cw.scale(1)
	avail := x1 - x0

	// Words, with long paths split after their slashes.
	var toks []token
	for _, sp := range spans {
		for _, piece := range tokenRE.FindAllString(sp.Text, -1) {
			w, _ := measure(piece, fontOf(sp.Style))
			if sp.Style == Code && w > avail {
				start := 0
				for i := 0; i < len(piece); i++ {
					if piece[i] == '\\' || piece[i] == '/' || i == len(piece)-1 {
						part := piece[start : i+1]
						pw, _ := measure(part, code)
						toks = append(toks, token{part, Code, pw})
						start = i + 1
					}
				}
				continue
			}
			toks = append(toks, token{piece, sp.Style, w})
		}
	}
	// Lines.
	var lines [][]token
	var cur []token
	x := int32(0)
	trailing := func(t token) int32 { // width without the space after it
		trim := len(t.s)
		for trim > 0 && t.s[trim-1] == ' ' {
			trim--
		}
		if trim == len(t.s) {
			return t.w
		}
		w, _ := measure(t.s[:trim], fontOf(t.style))
		return w
	}
	for _, t := range toks {
		if len(cur) > 0 && x+trailing(t) > avail {
			lines = append(lines, cur)
			cur, x = nil, 0
		}
		if len(cur) == 0 && len(t.s) > 0 && t.s[0] == ' ' {
			continue
		}
		cur = append(cur, t)
		x += t.w
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	if dc != 0 {
		pSetBkMode.Call(uintptr(dc), 1)
		for i, line := range lines {
			lw := int32(0)
			for j, t := range line {
				if j == len(line)-1 {
					lw += trailing(t)
				} else {
					lw += t.w
				}
			}
			lx := x0
			if center {
				lx = x0 + (avail-lw)/2
			}
			ly := y + int32(i)*lineH
			for _, t := range line {
				f := fontOf(t.style)
				if t.style == Code {
					tw := trailing(t)
					bg, _, _ := pCreateSolidBrush.Call(colCodeBack)
					r := rect{lx - cw.scale(2), ly, lx + tw + cw.scale(2), ly + lineH}
					pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&r)), bg)
					pDeleteObject.Call(bg)
				}
				old, _, _ := pSelectObject.Call(uintptr(dc), uintptr(f))
				pSetTextColor.Call(uintptr(dc), color)
				u := windows.StringToUTF16(t.s)
				pTextOutW.Call(uintptr(dc), uintptr(lx), uintptr(ly), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1))
				pSelectObject.Call(uintptr(dc), old)
				lx += t.w
			}
		}
	}
	return y + int32(len(lines))*lineH
}

// glyph draws one Font Awesome icon centered in r.
func (cw *CardWindow) glyph(dc windows.Handle, g rune, f windows.Handle, color uintptr, r rect) {
	if dc == 0 || !haveGlyph || g == 0 {
		return
	}
	old, _, _ := pSelectObject.Call(uintptr(dc), uintptr(f))
	pSetTextColor.Call(uintptr(dc), color)
	pSetBkMode.Call(uintptr(dc), 1)
	pDrawTextW.Call(uintptr(dc), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(string(g)))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), dtCenter|dtVCenter|dtSingle|dtNoPrefix)
	pSelectObject.Call(uintptr(dc), old)
}

// box draws a rounded rectangle; fill ^0 leaves the inside alone.
func (cw *CardWindow) box(dc windows.Handle, r rect, fill, border uintptr, round int32) {
	pen, _, _ := pCreatePen.Call(0, uintptr(max(1, cw.scale(3)/2)), border)
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

func (cw *CardWindow) line(dc windows.Handle, color uintptr, x, y, toX int32) {
	if dc == 0 {
		return
	}
	pen, _, _ := pCreatePen.Call(0, uintptr(max(1, cw.scale(3)/2)), color)
	old, _, _ := pSelectObject.Call(uintptr(dc), pen)
	pMoveToEx.Call(uintptr(dc), uintptr(x), uintptr(y), 0)
	pLineTo.Call(uintptr(dc), uintptr(toX), uintptr(y))
	pSelectObject.Call(uintptr(dc), old)
	pDeleteObject.Call(pen)
}

// buttonWidth is a button's width for its words and icon.
func (cw *CardWindow) buttonWidth(b Button) int32 {
	d, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, d)
	f := cw.fonts.body
	if b.Main {
		f = cw.fonts.bold
	}
	old, _, _ := pSelectObject.Call(d, uintptr(f))
	u := windows.StringToUTF16(b.Label)
	var sz struct{ cx, cy int32 }
	pGetTextExtentPoint32W.Call(d, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	pSelectObject.Call(d, old)
	w := sz.cx + cw.scale(32)
	if b.Icon != 0 && haveGlyph {
		w += cw.scale(22)
	}
	return w
}

// layout places everything; with a dc it also paints. It returns the client
// size. The first call (dc 0) records where the buttons and links go.
func (cw *CardWindow) layout(dc windows.Handle) (width, height int32) {
	width = cw.scale(cdWidth)
	x0, x1 := cw.scale(cdOuter+cdInner), width-cw.scale(cdOuter+cdInner)
	frameTop := cw.scale(cdOuter) + cw.scale(4)
	y := frameTop + cw.scale(cdTop) - cw.scale(6)
	accent := uintptr(colGreen)
	if cw.card.Bad {
		accent = colBad
	}
	measuring := dc == 0
	if measuring {
		cw.places, cw.links = nil, nil
	}
	body := func(spans []Span, from, to, y int32, center bool, color uintptr) int32 {
		return cw.flow(dc, spans, cw.fonts.body, cw.fonts.bold, cw.fonts.code, from, to, y, center, color)
	}

	// Banners first, then the title, then the rest in order.
	for _, b := range cw.card.Blocks {
		if b.Kind != BlockBanner {
			continue
		}
		back, text, icon := uintptr(colOKBack), uintptr(colOKText), uintptr(colGreen)
		if b.Warn {
			back, text, icon = colWarnBack, colWarnText, colWarnIcon
		}
		pad := cw.scale(9)
		bottom := cw.flow(0, b.Text, cw.fonts.body, cw.fonts.bold, cw.fonts.code, x0+pad+cw.scale(24), x1-pad, y+pad, false, text) + pad
		if !measuring {
			cw.box(dc, rect{x0, y, x1, bottom}, back, back, cw.scale(10))
			cw.glyph(dc, b.Icon, cw.fonts.icon, icon, rect{x0 + pad, y + pad, x0 + pad + cw.scale(20), y + pad + cw.scale(20)})
			body(b.Text, x0+pad+cw.scale(24), x1-pad, y+pad, false, text)
		}
		y = bottom + cw.scale(14)
	}
	y = cw.flow(dc, []Span{{cw.card.Title, Plain}}, cw.fonts.title, cw.fonts.title, cw.fonts.title, x0, x1, y, true, colTitle) + cw.scale(6)

	for _, b := range cw.card.Blocks {
		switch b.Kind {
		case BlockSub:
			y = body(b.Text, x0, x1, y, true, colText) + cw.scale(14)
		case BlockButtons:
			h := cw.scale(cdButtonH)
			for _, bt := range b.Buttons {
				if bt.Main {
					h = cw.scale(cdMainH)
				}
			}
			gap := cw.scale(8)
			// Rows of buttons that fit, each centered.
			var row []Button
			var rowW int32
			flush := func() {
				x := x0 + (x1-x0-rowW)/2
				for _, bt := range row {
					w := cw.buttonWidth(bt)
					if measuring {
						cw.places = append(cw.places, placed{id: bt.ID, r: rect{x, y, x + w, y + h}, main: bt.Main, icon: bt.Icon, kind: BlockButtons})
						cw.label[bt.ID] = bt.Label
					}
					x += w + gap
				}
				y += h + gap
				row, rowW = nil, 0
			}
			for _, bt := range b.Buttons {
				w := cw.buttonWidth(bt)
				if len(row) > 0 && rowW+gap+w > x1-x0 {
					flush()
				}
				if len(row) > 0 {
					rowW += gap
				}
				row = append(row, bt)
				rowW += w
			}
			flush()
			y += cw.scale(4)
		case BlockRows:
			for _, r := range b.Rows {
				ix := rect{x0, y + cw.scale(1), x0 + cw.scale(cdIconCol) - cw.scale(6), y + cw.scale(19)}
				if !measuring {
					if r.AppIcon && cw.app != 0 {
						n := cw.scale(16)
						pDrawIconEx.Call(uintptr(dc), uintptr(ix.left+(ix.right-ix.left-n)/2), uintptr(y+cw.scale(2)), uintptr(cw.app), uintptr(n), uintptr(n), 0, 0, 3)
					} else {
						cw.glyph(dc, r.Icon, cw.fonts.icon, accent, ix)
					}
				}
				y = body(r.Text, x0+cw.scale(cdIconCol), x1, y, false, colText) + cw.scale(8)
			}
			y += cw.scale(4)
		case BlockCheck:
			h := cw.scale(24)
			if measuring {
				cw.places = append(cw.places, placed{id: b.ID, r: rect{x0, y - cw.scale(3), x1, y - cw.scale(3) + h}, icon: b.Icon, kind: BlockCheck})
				cw.check[b.ID] = b.Checked
				cw.label[b.ID] = spansText(b.Text)
			}
			y += h + cw.scale(8)
		case BlockDivider:
			y += cw.scale(4)
			mid, half := width/2, cw.scale(16)
			cw.line(dc, colRule, x0, y+cw.scale(8), mid-half)
			cw.line(dc, colRule, mid+half, y+cw.scale(8), x1)
			cw.glyph(dc, faSeedling, cw.fonts.icon, colGreen, rect{mid - half, y, mid + half, y + cw.scale(17)})
			y += cw.scale(28)
		case BlockHead:
			y = cw.flow(dc, b.Text, cw.fonts.head, cw.fonts.head, cw.fonts.head, x0, x1, y, false, colTitle) + cw.scale(8)
		case BlockLink:
			bottom := body(b.Text, x0, x1, y, true, colLink)
			if measuring {
				cw.links = append(cw.links, placed{id: b.ID, r: rect{x0, y, x1, bottom}})
			} else {
				// Underline under the words only.
				d, _, _ := pGetDC.Call(0)
				old, _, _ := pSelectObject.Call(d, uintptr(cw.fonts.body))
				u := windows.StringToUTF16(spansText(b.Text))
				var sz struct{ cx, cy int32 }
				pGetTextExtentPoint32W.Call(d, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
				pSelectObject.Call(d, old)
				pReleaseDC.Call(0, d)
				mid := width / 2
				cw.line(dc, colLink, mid-sz.cx/2, bottom-cw.scale(2), mid+sz.cx/2)
			}
			y = bottom + cw.scale(8)
		case BlockDetails:
			pad := cw.scale(7)
			bottom := cw.flow(0, b.Text, cw.fonts.small, cw.fonts.small, cw.fonts.small, x0+pad, x1-pad, y+pad, false, colGrey) + pad
			if !measuring {
				cw.box(dc, rect{x0, y, x1, bottom}, colDetails, colDetails, cw.scale(10))
				cw.flow(dc, b.Text, cw.fonts.small, cw.fonts.small, cw.fonts.small, x0+pad, x1-pad, y+pad, false, colGrey)
			}
			y = bottom + cw.scale(6)
		}
	}
	frameBottom := y + cw.scale(cdInner) - cw.scale(8)
	height = frameBottom + cw.scale(cdOuter)
	if !measuring {
		border := uintptr(colRule)
		if cw.card.Bad {
			border = 0x00ADB2D9 // #d9b2ad
		}
		cw.box(dc, rect{cw.scale(cdOuter), frameTop, width - cw.scale(cdOuter), frameBottom}, ^uintptr(0), border, cw.scale(18))
		if haveGlyph {
			gap := rect{width/2 - cw.scale(20), frameTop - cw.scale(12), width/2 + cw.scale(20), frameTop + cw.scale(12)}
			paper, _, _ := pCreateSolidBrush.Call(colPaper)
			pFillRect.Call(uintptr(dc), uintptr(unsafe.Pointer(&gap)), paper)
			pDeleteObject.Call(paper)
			cw.glyph(dc, faSeedling, cw.fonts.seed, accent, gap)
		}
	}
	return width, height
}

// drawKid paints a button or the check box.
func (cw *CardWindow) drawKid(d *drawItem) {
	var p placed
	for _, q := range cw.places {
		if q.id == int(d.ctlID) {
			p = q
		}
	}
	cardMu.Lock()
	label, on := cw.label[p.id], cw.check[p.id]
	cardMu.Unlock()
	paper, _, _ := pCreateSolidBrush.Call(colPaper)
	pFillRect.Call(uintptr(d.hdc), uintptr(unsafe.Pointer(&d.item)), paper)
	pDeleteObject.Call(paper)
	pressed := d.itemState&odsSelected != 0
	r := d.item
	if p.kind == BlockCheck {
		accent := uintptr(colGreen)
		cw.glyph(d.hdc, p.icon, cw.fonts.icon, accent, rect{r.left, r.top + cw.scale(3), r.left + cw.scale(cdIconCol) - cw.scale(6), r.top + cw.scale(21)})
		bx := r.left + cw.scale(cdIconCol)
		n := cw.scale(16)
		by := r.top + (r.bottom-r.top-n)/2
		fill := uintptr(colWhite)
		if on {
			fill = colGreen
		}
		cw.box(d.hdc, rect{bx, by, bx + n, by + n}, fill, colGreen, cw.scale(5))
		if on {
			cw.glyph(d.hdc, faCheck, cw.fonts.icon, colWhite, rect{bx, by, bx + n, by + n})
		}
		cw.flow(d.hdc, []Span{{label, Bold}}, cw.fonts.bold, cw.fonts.bold, cw.fonts.bold, bx+n+cw.scale(8), r.right, r.top+(r.bottom-r.top-cw.scale(18))/2, false, colTitle)
		return
	}
	fill, border, text, icon, f := uintptr(colButton), uintptr(colBorder), uintptr(colDark), uintptr(colGreen), cw.fonts.body
	if p.main {
		fill, border, text, icon, f = colGreen, colGreen, colWhite, colWhite, cw.fonts.bold
		if pressed {
			fill, border = 0x00358015, 0x00358015 // #158035
		}
	} else if pressed {
		fill = colPress
	}
	cw.box(d.hdc, r, fill, border, cw.scale(10))
	measure := func(s string, ft windows.Handle) int32 {
		old, _, _ := pSelectObject.Call(uintptr(d.hdc), uintptr(ft))
		defer pSelectObject.Call(uintptr(d.hdc), old)
		u := windows.StringToUTF16(s)
		var sz struct{ cx, cy int32 }
		pGetTextExtentPoint32W.Call(uintptr(d.hdc), uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
		return sz.cx
	}
	gw := int32(0)
	if p.icon != 0 && haveGlyph {
		gw = measure(string(p.icon), cw.fonts.icon) + cw.scale(7)
	}
	tw := measure(label, f)
	x := r.left + (r.right-r.left-gw-tw)/2
	pSetBkMode.Call(uintptr(d.hdc), 1)
	if gw > 0 {
		cw.glyph(d.hdc, p.icon, cw.fonts.icon, icon, rect{x, r.top, x + gw - cw.scale(7), r.bottom})
	}
	old, _, _ := pSelectObject.Call(uintptr(d.hdc), uintptr(f))
	pSetTextColor.Call(uintptr(d.hdc), text)
	tr := rect{x + gw, r.top, x + gw + tw + cw.scale(2), r.bottom}
	pDrawTextW.Call(uintptr(d.hdc), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(label))), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), dtSingle|dtVCenter|dtNoPrefix)
	pSelectObject.Call(uintptr(d.hdc), old)
}

func (cw *CardWindow) overLink() int {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	pScreenToClient.Call(uintptr(cw.hwnd), uintptr(unsafe.Pointer(&p)))
	for _, l := range cw.links {
		if p.x >= l.r.left && p.x < l.r.right && p.y >= l.r.top && p.y < l.r.bottom {
			return l.id
		}
	}
	return 0
}

func (cw *CardWindow) dismiss() {
	if cw.ev.Close != nil {
		cw.ev.Close(cw)
		return
	}
	cw.result = cw.dismissID()
	pDestroyWindow.Call(uintptr(cw.hwnd))
}

func (cw *CardWindow) pressed(id int) {
	if cw.ev.Button == nil || cw.ev.Button(cw, id) {
		cardMu.Lock()
		cw.result = id
		cardMu.Unlock()
		pDestroyWindow.Call(uintptr(cw.hwnd))
	}
}

func cardWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	cardMu.Lock()
	cw := cards[windows.Handle(hwnd)]
	cardMu.Unlock()
	if cw != nil {
		switch message {
		case wmPaint:
			var ps paintStruct
			hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			cw.layout(windows.Handle(hdc))
			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		case wmDrawItem:
			cw.drawKid(*(**drawItem)(unsafe.Pointer(&lParam)))
			return 1
		case wmCommand:
			id := int(wParam & 0xffff)
			for _, p := range cw.places {
				if p.id != id {
					continue
				}
				if p.kind == BlockCheck {
					cardMu.Lock()
					cw.check[id] = !cw.check[id]
					on := cw.check[id]
					h := cw.kids[id]
					cardMu.Unlock()
					pInvalidateRect.Call(uintptr(h), 0, 1)
					if cw.ev.Check != nil {
						cw.ev.Check(cw, id, on)
					}
				} else {
					cw.pressed(id)
				}
				return 0
			}
		case wmSetCursor:
			if cw.overLink() != 0 {
				hand, _, _ := pLoadCursorW.Call(0, 32649) // IDC_HAND
				pSetCursor.Call(hand)
				return 1
			}
		case wmLButtonUp:
			if id := cw.overLink(); id != 0 {
				cw.pressed(id)
				return 0
			}
		case wmSysCommand:
			if wParam&0xFFF0 == scMinimize {
				cw.dismiss()
				return 0
			}
		case wmClose:
			cw.dismiss()
			return 0
		case wmCardClose:
			cardMu.Lock()
			cw.result = int(wParam)
			cardMu.Unlock()
			pDestroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			pPostQuitMessage.Call(0)
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

// CopyText puts s on the clipboard (Copy details).
func CopyText(s string) error { return copyText(s) }

// OpenFolder shows dir in File Explorer.
func OpenFolder(dir string) error { return exec.Command("explorer", dir).Start() }
