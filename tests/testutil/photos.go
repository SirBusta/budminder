package testutil

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/require"
)

// PhonePhotoJPEG is a small JPEG shaped like a phone photo: an EXIF block
// with the camera model (secret), a GPS block and orientation 6 (rotated),
// an XMP block and a comment that also carry secret, and secret again after
// the end of the image (where phones append motion photos). Tests check
// that BudMinder stores none of it except the orientation.
func PhonePhotoJPEG(t *testing.T, secret string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for x := range 16 {
		for y := range 12 {
			img.Set(x, y, color.RGBA{uint8(x * 15), uint8(y * 20), 90, 255})
		}
	}
	var enc bytes.Buffer
	require.NoError(t, jpeg.Encode(&enc, img, nil))
	plain := enc.Bytes()

	le := binary.LittleEndian
	entry := func(tag, typ uint16, count, value uint32) []byte {
		e := make([]byte, 12)
		le.PutUint16(e, tag)
		le.PutUint16(e[2:], typ)
		le.PutUint32(e[4:], count)
		le.PutUint32(e[8:], value)
		return e
	}
	modelOff := uint32(8 + 2 + 3*12 + 4)
	gpsOff := modelOff + uint32(len(secret)+1)
	tiff := []byte("II\x2a\x00\x08\x00\x00\x00\x03\x00")
	tiff = append(tiff, entry(0x0110, 2, uint32(len(secret)+1), modelOff)...) // Model
	tiff = append(tiff, entry(0x0112, 3, 1, 6)...)                           // Orientation
	tiff = append(tiff, entry(0x8825, 4, 1, gpsOff)...)                      // GPS IFD
	tiff = append(tiff, 0, 0, 0, 0)
	tiff = append(tiff, secret+"\x00"...)
	tiff = append(tiff, 1, 0)
	tiff = append(tiff, entry(0x0001, 2, 2, 'N')...) // GPSLatitudeRef
	tiff = append(tiff, 0, 0, 0, 0)

	segment := func(marker byte, payload []byte) []byte {
		s := []byte{0xFF, marker, 0, 0}
		binary.BigEndian.PutUint16(s[2:], uint16(len(payload)+2))
		return append(s, payload...)
	}
	var out []byte
	out = append(out, plain[:2]...)
	out = append(out, segment(0xE1, append([]byte("Exif\x00\x00"), tiff...))...)
	out = append(out, segment(0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>"+secret+"</x:xmpmeta>"))...)
	out = append(out, segment(0xFE, []byte(secret))...)
	out = append(out, plain[2:]...)
	return append(out, secret...)
}

// DetailedPhotoJPEG is a 320x240 photo-like JPEG saved at quality 100, so
// compressing it (Settings > Photos) saves well over 10%. It carries
// orientation 6 like a phone photo after metadata stripping.
func DetailedPhotoJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for x := range 320 {
		for y := range 240 {
			n := uint8((x*7 + y*13 + (x*y)%31) % 256)
			img.Set(x, y, color.RGBA{n, uint8(x), uint8(y), 255})
		}
	}
	var enc bytes.Buffer
	require.NoError(t, jpeg.Encode(&enc, img, &jpeg.Options{Quality: 100}))
	plain := enc.Bytes()
	exif := []byte("Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08\x00\x01\x01\x12\x00\x03\x00\x00\x00\x01\x00\x06\x00\x00\x00\x00\x00\x00")
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(exif)+2))
	out := append([]byte{}, plain[:2]...)
	out = append(out, append(seg, exif...)...)
	return append(out, plain[2:]...)
}
