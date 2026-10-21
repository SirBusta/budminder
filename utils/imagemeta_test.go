package utils

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/webp"
)

// secret is planted in every kind of metadata; none of it may survive.
const secret = "SECRET-PHONE-GPS-37.7749N"

func testPicture() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for x := 0; x < 16; x++ {
		for y := 0; y < 12; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 15), uint8(y * 20), 90, 255})
		}
	}
	return img
}

func jpegSegment(marker byte, payload []byte) []byte {
	s := []byte{0xFF, marker, 0, 0}
	binary.BigEndian.PutUint16(s[2:], uint16(len(payload)+2))
	return append(s, payload...)
}

// exifBlock is an EXIF APP1 payload (little-endian, like most phones) with
// the orientation, a camera model and a GPS IFD.
func exifBlock(orientation uint16) []byte {
	t := []byte("II\x2a\x00\x08\x00\x00\x00")
	le := binary.LittleEndian
	entry := func(tag, typ uint16, count, value uint32) []byte {
		e := make([]byte, 12)
		le.PutUint16(e, tag)
		le.PutUint16(e[2:], typ)
		le.PutUint32(e[4:], count)
		le.PutUint32(e[8:], value)
		return e
	}
	// IFD0 at 8: 3 entries (2+36+4 = 42 bytes), then the model string, then GPS IFD.
	modelOff := uint32(8 + 2 + 3*12 + 4)
	gpsOff := modelOff + uint32(len(secret)+1)
	t = append(t, 3, 0)
	t = append(t, entry(0x0110, 2, uint32(len(secret)+1), modelOff)...) // Model
	t = append(t, entry(0x0112, 3, 1, uint32(orientation))...)          // Orientation
	t = append(t, entry(0x8825, 4, 1, gpsOff)...)                       // GPS IFD
	t = append(t, 0, 0, 0, 0)
	t = append(t, secret+"\x00"...)
	t = append(t, 1, 0)
	t = append(t, entry(0x0001, 2, 2, 'N')...) // GPSLatitudeRef
	t = append(t, 0, 0, 0, 0)
	return append([]byte("Exif\x00\x00"), t...)
}

func TestStripJPEG(t *testing.T) {
	t.Parallel()
	var enc bytes.Buffer
	require.NoError(t, jpeg.Encode(&enc, testPicture(), nil))
	plain := enc.Bytes()

	icc := append([]byte("ICC_PROFILE\x00\x01\x01"), "fake sRGB profile"...)
	var in []byte
	in = append(in, plain[:2]...)
	in = append(in, jpegSegment(0xE1, exifBlock(6))...)
	in = append(in, jpegSegment(0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>"+secret+"</x:xmpmeta>"))...)
	in = append(in, jpegSegment(0xE2, icc)...)
	in = append(in, jpegSegment(0xED, []byte("Photoshop 3.0\x00"+secret))...)
	in = append(in, jpegSegment(0xFE, []byte(secret))...)
	in = append(in, plain[2:]...)
	in = append(in, []byte("MotionPhoto_Data "+secret)...) // after EOI

	out, err := StripImageMetadata(in)
	require.NoError(t, err)
	assert.NotContains(t, string(out), secret, "no camera, location, XMP, IPTC, comment or trailing data")
	assert.Contains(t, string(out), "fake sRGB profile", "the color profile stays")
	assert.Equal(t, []byte{0xFF, 0xD9}, out[len(out)-2:], "ends at the image's end")

	i := bytes.Index(out, []byte("Exif\x00\x00"))
	require.GreaterOrEqual(t, i, 0, "orientation kept in a minimal EXIF block")
	assert.Equal(t, 6, exifOrientation(out[i+6:]))

	a, err := jpeg.Decode(bytes.NewReader(plain))
	require.NoError(t, err)
	b, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, a, b, "the picture itself is untouched")

	// No orientation (or the normal one): no EXIF block at all.
	in2 := append(append(append([]byte{}, plain[:2]...), jpegSegment(0xE1, exifBlock(1))...), plain[2:]...)
	out2, err := StripImageMetadata(in2)
	require.NoError(t, err)
	assert.NotContains(t, string(out2), "Exif")
}

func TestStripJPEG_Progressive(t *testing.T) {
	t.Parallel()
	// A real progressive JPEG has several scans with tables between them.
	_, file, _, _ := runtime.Caller(0)
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "data", "demo", "smoke_photos", "*.jpg"))
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		require.NoError(t, err)
		out, err := StripImageMetadata(raw)
		require.NoError(t, err, m)
		a, err := jpeg.Decode(bytes.NewReader(raw))
		require.NoError(t, err)
		b, err := jpeg.Decode(bytes.NewReader(out))
		require.NoError(t, err, m)
		assert.Equal(t, a.Bounds(), b.Bounds(), m)
	}
}

func pngChunk(kind string, data []byte) []byte {
	c := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(c, uint32(len(data)))
	copy(c[4:], kind)
	c = append(c, data...)
	crc := crc32.ChecksumIEEE(c[4:])
	return binary.BigEndian.AppendUint32(c, crc)
}

func TestStripPNG(t *testing.T) {
	t.Parallel()
	var enc bytes.Buffer
	require.NoError(t, png.Encode(&enc, testPicture()))
	plain := enc.Bytes()
	ihdrEnd := 8 + 12 + 13

	var in []byte
	in = append(in, plain[:ihdrEnd]...)
	in = append(in, pngChunk("sRGB", []byte{0})...)
	in = append(in, pngChunk("tEXt", []byte("Comment\x00"+secret))...)
	in = append(in, pngChunk("eXIf", exifBlock(6)[6:])...)
	in = append(in, pngChunk("iTXt", []byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00"+secret))...)
	in = append(in, pngChunk("tIME", []byte{7, 234, 10, 21, 9, 0, 0})...)
	in = append(in, plain[ihdrEnd:]...)
	in = append(in, secret...) // after IEND

	out, err := StripImageMetadata(in)
	require.NoError(t, err)
	assert.NotContains(t, string(out), secret)
	assert.Contains(t, string(out), "sRGB", "the color space stays")
	assert.NotContains(t, string(out), "tIME")
	a, _ := png.Decode(bytes.NewReader(plain))
	b, err := png.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, a, b)
}

func webpChunk(kind string, data []byte) []byte {
	c := make([]byte, 8, 8+len(data)+1)
	copy(c, kind)
	binary.LittleEndian.PutUint32(c[4:], uint32(len(data)))
	c = append(c, data...)
	if len(data)%2 == 1 {
		c = append(c, 0)
	}
	return c
}

func TestStripWebP(t *testing.T) {
	t.Parallel()
	_, file, _, _ := runtime.Caller(0)
	simple, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "web", "static", "img", "manual", "plant-photo-light.webp"))
	require.NoError(t, err)
	cfg, err := webp.DecodeConfig(bytes.NewReader(simple))
	require.NoError(t, err)
	riff := int(binary.LittleEndian.Uint32(simple[4:8]))
	frame := simple[12 : 8+riff] // the VP8/VP8L chunk

	vp8x := make([]byte, 10)
	vp8x[0] = 0x20 | 0x08 | 0x04 // ICC, EXIF, XMP
	w, h := cfg.Width-1, cfg.Height-1
	vp8x[4], vp8x[5], vp8x[6] = byte(w), byte(w>>8), byte(w>>16)
	vp8x[7], vp8x[8], vp8x[9] = byte(h), byte(h>>8), byte(h>>16)
	body := []byte("WEBP")
	body = append(body, webpChunk("VP8X", vp8x)...)
	body = append(body, webpChunk("ICCP", []byte("fake sRGB profile"))...)
	body = append(body, frame...)
	body = append(body, webpChunk("EXIF", exifBlock(6)[6:])...)
	body = append(body, webpChunk("XMP ", []byte("<x:xmpmeta>"+secret+"</x:xmpmeta>"))...)
	in := append([]byte("RIFF\x00\x00\x00\x00"), body...)
	binary.LittleEndian.PutUint32(in[4:], uint32(len(body)))
	in = append(in, secret...) // after the RIFF

	out, err := StripImageMetadata(in)
	require.NoError(t, err)
	assert.NotContains(t, string(out), secret)
	assert.Contains(t, string(out), "fake sRGB profile")
	assert.Equal(t, len(out)-8, int(binary.LittleEndian.Uint32(out[4:8])), "RIFF size matches")
	flags := out[12+8]
	assert.Zero(t, flags&(0x08|0x04), "VP8X no longer claims EXIF or XMP")
	assert.NotZero(t, flags&0x20, "but still the color profile")
	got, err := webp.DecodeConfig(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, cfg.Width, got.Width)
}

func TestStripGIF(t *testing.T) {
	t.Parallel()
	pal := image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black, color.White})
	pal.SetColorIndex(3, 3, 1)
	var enc bytes.Buffer
	require.NoError(t, gif.EncodeAll(&enc, &gif.GIF{Image: []*image.Paletted{pal, pal}, Delay: []int{10, 10}, LoopCount: 0}))
	plain := enc.Bytes()
	head := 13 // header, plus the global color table if there is one
	if plain[10]&0x80 != 0 {
		head += 3 << (int(plain[10]&7) + 1)
	}

	comment := append([]byte{0x21, 0xFE, byte(len(secret))}, append([]byte(secret), 0)...)
	xmp := append([]byte{0x21, 0xFF, 11}, "XMP DataXMP"...)
	xmp = append(append(append(xmp, byte(len(secret))), secret...), 0)
	var in []byte
	in = append(in, plain[:head]...)
	in = append(in, comment...)
	in = append(in, xmp...)
	in = append(in, plain[head:]...)
	in = append(in, secret...) // after the trailer

	out, err := StripImageMetadata(in)
	require.NoError(t, err)
	assert.NotContains(t, string(out), secret)
	assert.Contains(t, string(out), "NETSCAPE2.0", "looping stays")
	g, err := gif.DecodeAll(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Len(t, g.Image, 2, "both frames")
}

func TestStripImageMetadata_RefusesOthers(t *testing.T) {
	t.Parallel()
	for name, b := range map[string][]byte{
		"text":           []byte("hello"),
		"truncated JPEG": {0xFF, 0xD8, 0xFF, 0xE1, 0x10},
		"no JPEG end":    append([]byte{0xFF, 0xD8}, jpegSegment(0xFE, []byte("x"))...),
		"truncated PNG":  []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"),
		"bad WebP size":  []byte("RIFF\xff\xff\xff\x7fWEBP"),
	} {
		_, err := StripImageMetadata(b)
		assert.ErrorIs(t, err, ErrImageFormat, name)
	}
}
