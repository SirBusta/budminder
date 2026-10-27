package utils

// CompressJPEG re-encodes a JPEG photo at a lower quality to save space
// (Settings > Photos). The pixel size stays the same. Go's encoder writes a
// bare JPEG, so the two things StripImageMetadata keeps for how a photo
// looks are put back: the orientation tag (pixels are not rotated, so a
// sideways phone photo still shows upright) and the ICC color profile.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"
)

// ErrNotCompressible means the photo isn't a JPEG BudMinder can re-encode
// without changing how it looks (PNG, GIF, WebP, CMYK JPEGs, damaged files).
// Callers keep the original.
var ErrNotCompressible = errors.New("image: not a JPEG that can be compressed")

// MinPhotoSaving is the share a compressed photo must save to replace the
// original (10%); anything less isn't worth the quality.
const MinPhotoSaving = 10

// IsJPEG reports whether b starts like a JPEG file.
func IsJPEG(b []byte) bool {
	return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF
}

// CompressJPEG returns b re-encoded at quality (1-100), keeping its
// orientation tag and color profile.
func CompressJPEG(b []byte, quality int) ([]byte, error) {
	if !IsJPEG(b) {
		return nil, ErrNotCompressible
	}
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, ErrNotCompressible
	}
	switch img.(type) {
	case *image.YCbCr, *image.Gray:
	default: // CMYK and the like: Go would write them back as different colors
		return nil, ErrNotCompressible
	}
	var out bytes.Buffer
	out.Grow(len(b) / 2)
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	icc, orientation := jpegLook(b)
	enc := out.Bytes()
	if len(icc) > 0 {
		with := make([]byte, 0, len(enc)+len(icc))
		with = append(with, enc[:2]...) // SOI
		with = append(with, icc...)
		enc = append(with, enc[2:]...)
	}
	if orientation > 1 {
		enc = withOrientation(enc, orientation) // goes before the profile
	}
	return enc, nil
}

// ShouldReplace reports whether a compressed photo of comp bytes saves
// enough over the original's orig bytes (MinPhotoSaving) to replace it.
func ShouldReplace(orig, comp int) bool {
	return orig > 0 && comp > 0 && int64(comp)*100 <= int64(orig)*(100-MinPhotoSaving)
}

// jpegLook returns the ICC profile segments (APP2 "ICC_PROFILE", in order,
// markers included) and the EXIF orientation of a JPEG, read up to its image
// data.
func jpegLook(b []byte) (icc []byte, orientation int) {
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return icc, orientation
		}
		marker := b[i+1]
		if marker == 0xFF {
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // image data starts: no more metadata
			return icc, orientation
		}
		if marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 {
			i += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(b[i+2:]))
		end := i + 2 + size
		if size < 2 || end > len(b) {
			return icc, orientation
		}
		payload := b[i+4 : end]
		switch {
		case marker == 0xE2 && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")):
			icc = append(icc, b[i:end]...)
		case marker == 0xE1 && orientation == 0 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")):
			orientation = exifOrientation(payload[6:])
		}
		i = end
	}
	return icc, orientation
}

// ThumbnailJPEG scales a JPEG down to width pixels wide (the picker on the
// compare page), keeping its orientation tag and color profile.
func ThumbnailJPEG(b []byte, width int) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, ErrNotCompressible
	}
	r := img.Bounds()
	if r.Dx() > width {
		h := max(1, r.Dy()*width/max(1, r.Dx()))
		dst := image.NewRGBA(image.Rect(0, 0, width, h))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, r, draw.Src, nil)
		img = dst
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	icc, orientation := jpegLook(b)
	enc := out.Bytes()
	if len(icc) > 0 {
		enc = append(append(append([]byte{}, enc[:2]...), icc...), enc[2:]...)
	}
	if orientation > 1 {
		enc = withOrientation(enc, orientation)
	}
	return enc, nil
}
