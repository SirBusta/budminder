package launcher

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/draw"
)

// trayIconSizes are the sizes in the tray icon's .ico: Windows picks the one
// that fits the screen's scaling (16 at 100%, 20 at 125%, 24 at 150%, 32 at
// 200%) instead of shrinking the 256 picture itself, which looks blurry.
var trayIconSizes = []int{16, 20, 24, 32, 48}

// amber is the "Update available" pill's color (web/static/css).
var amber = color.NRGBA{R: 0xf5, G: 0x9e, B: 0x0b, A: 0xff}

// TrayIcon is the tray icon as an .ico file, made from the 256x256 PNG.
// border puts an amber frame around it: an update is available.
func TrayIcon(src []byte, border bool) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	var pngs [][]byte
	for _, n := range trayIconSizes {
		b, err := encodePNG(iconAt(img, n, border))
		if err != nil {
			return nil, err
		}
		pngs = append(pngs, b)
	}
	return icoOf(pngs, trayIconSizes), nil
}

// IconPNG is the icon at n x n pixels as a PNG (the phone window's title bar).
func IconPNG(src []byte, n int) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	return encodePNG(iconAt(img, n, false))
}

// iconAt shrinks the icon to n x n. With border, the icon's rounded square is
// filled amber and a smaller icon goes on top, leaving an amber frame that
// follows the rounded corners (2 pixels at 16, wider on bigger icons).
func iconAt(img image.Image, n int, border bool) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	if !border {
		draw.CatmullRom.Scale(out, out.Bounds(), img, img.Bounds(), draw.Over, nil)
		return out
	}
	shape := image.NewNRGBA(out.Bounds())
	draw.CatmullRom.Scale(shape, shape.Bounds(), img, img.Bounds(), draw.Over, nil)
	draw.DrawMask(out, out.Bounds(), &image.Uniform{amber}, image.Point{}, shape, image.Point{}, draw.Over)
	k := max(2, n/10)
	draw.CatmullRom.Scale(out, image.Rect(k, k, n-k, n-k), img, img.Bounds(), draw.Over, nil)
	return out
}

func encodePNG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := png.Encode(&b, img)
	return b.Bytes(), err
}

// icoOf puts PNG images (each sizes[i] square) into one .ico file; Windows
// Vista and later read PNG-compressed icons.
func icoOf(pngs [][]byte, sizes []int) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&b, le, [3]uint16{0, 1, uint16(len(pngs))}) // reserved, type 1 = icon, count
	offset := 6 + 16*len(pngs)
	for i, p := range pngs {
		side := byte(sizes[i])                                              // 256 is written as 0
		b.Write([]byte{side, side, 0, 0})                                   // width, height, no palette, reserved
		_ = binary.Write(&b, le, [2]uint16{1, 32})                          // planes, bits per pixel
		_ = binary.Write(&b, le, [2]uint32{uint32(len(p)), uint32(offset)}) // size, where the image starts
		offset += len(p)
	}
	for _, p := range pngs {
		b.Write(p)
	}
	return b.Bytes()
}
