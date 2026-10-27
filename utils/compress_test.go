package utils

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeICC stands in for a color profile; only the bytes matter here.
var fakeICC = append([]byte("ICC_PROFILE\x00\x01\x01"), bytes.Repeat([]byte("prof"), 50)...)

// detailedJPEG is a photo-like JPEG saved at quality 100 (so compressing it
// saves a lot), with orientation 6 and a color profile, like a phone photo
// after StripImageMetadata.
func detailedJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewYCbCr(image.Rect(0, 0, 320, 240), image.YCbCrSubsampleRatio420)
	rgba := image.NewRGBA(img.Rect)
	for x := range 320 {
		for y := range 240 {
			n := uint8((x*7 + y*13 + (x*y)%31) % 256)
			rgba.Set(x, y, color.RGBA{n, uint8(x), uint8(y), 255})
		}
	}
	var enc bytes.Buffer
	require.NoError(t, jpeg.Encode(&enc, rgba, &jpeg.Options{Quality: 100}))
	plain := enc.Bytes()
	var out []byte
	out = append(out, plain[:2]...)
	out = append(out, jpegSegment(0xE1, exifBlock(6))...)
	out = append(out, jpegSegment(0xE2, fakeICC)...)
	out = append(out, plain[2:]...)
	stripped, err := StripImageMetadata(out)
	require.NoError(t, err)
	return stripped
}

func TestCompressJPEG_SmallerKeepsLook(t *testing.T) {
	t.Parallel()
	orig := detailedJPEG(t)
	comp, err := CompressJPEG(orig, 75)
	require.NoError(t, err)

	assert.Less(t, len(comp), len(orig)*9/10, "compressing a quality-100 photo at 75 saves well over 10%")
	assert.True(t, ShouldReplace(len(orig), len(comp)))

	icc, orientation := jpegLook(comp)
	assert.Equal(t, 6, orientation, "orientation tag kept")
	assert.Equal(t, jpegSegment(0xE2, fakeICC), icc, "color profile kept")

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(comp))
	require.NoError(t, err)
	assert.Equal(t, 320, cfg.Width)
	assert.Equal(t, 240, cfg.Height)
	_, err = jpeg.Decode(bytes.NewReader(comp))
	require.NoError(t, err, "the result is a readable JPEG")
}

func TestCompressJPEG_NoProfileNoOrientation(t *testing.T) {
	t.Parallel()
	var enc bytes.Buffer
	require.NoError(t, jpeg.Encode(&enc, testPicture(), &jpeg.Options{Quality: 100}))
	comp, err := CompressJPEG(enc.Bytes(), 60)
	require.NoError(t, err)
	icc, orientation := jpegLook(comp)
	assert.Empty(t, icc)
	assert.Equal(t, 0, orientation)
}

func TestCompressJPEG_RefusesOtherFormats(t *testing.T) {
	t.Parallel()
	var p bytes.Buffer
	require.NoError(t, png.Encode(&p, testPicture()))
	for name, b := range map[string][]byte{
		"png":     p.Bytes(),
		"gif":     []byte("GIF89a......"),
		"webp":    []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
		"damaged": {0xFF, 0xD8, 0xFF, 0xE0, 0x00},
	} {
		_, err := CompressJPEG(b, 75)
		assert.ErrorIs(t, err, ErrNotCompressible, name)
	}
}

func TestShouldReplace(t *testing.T) {
	t.Parallel()
	assert.True(t, ShouldReplace(1000, 900), "exactly 10% saved")
	assert.False(t, ShouldReplace(1000, 901), "under 10% saved")
	assert.False(t, ShouldReplace(1000, 1200), "bigger than the original")
	assert.False(t, ShouldReplace(0, 0))
	assert.True(t, ShouldReplace(5_000_000_000, 100))
}

func TestCompressJPEG_AlreadySmallIsKept(t *testing.T) {
	t.Parallel()
	// A photo saved at 50 doesn't shrink 10% when saved at 90: keep it.
	orig := detailedJPEG(t)
	low, err := CompressJPEG(orig, 50)
	require.NoError(t, err)
	again, err := CompressJPEG(low, 90)
	require.NoError(t, err)
	assert.False(t, ShouldReplace(len(low), len(again)))
}
