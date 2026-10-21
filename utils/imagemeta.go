package utils

// StripImageMetadata removes the hidden information photos carry (GPS
// location, camera make and serial, dates, editing history, comments) from
// every photo BudMinder takes in. Only what changes how the picture looks is
// kept: the color profile and, for JPEG, the orientation tag (phones save
// most photos sideways and rely on it). The picture data itself is copied
// byte for byte, never re-encoded, so nothing is lost. Anything stored after
// the end of the image (phones append motion photos and depth maps there,
// with their own location tags) is dropped too.

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// ErrImageFormat means the file isn't a JPEG, PNG, WebP or GIF that could be
// read safely; callers refuse it rather than store it unstripped.
var ErrImageFormat = errors.New("image: unsupported or damaged file")

// StripImageMetadata returns the image without its metadata (see above).
func StripImageMetadata(b []byte) ([]byte, error) {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return stripJPEG(b)
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return stripPNG(b)
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return stripWebP(b)
	case bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")):
		return stripGIF(b)
	}
	return nil, ErrImageFormat
}

// ---------------------------------------------------------------- JPEG

func stripJPEG(b []byte) ([]byte, error) {
	out := bytes.NewBuffer(make([]byte, 0, len(b)))
	out.Write(b[:2]) // SOI
	orientation := 0
	i := 2
	inScan := false
	for i < len(b) {
		if inScan {
			// Entropy-coded data: copy until the next real marker.
			start := i
			for i < len(b) {
				if b[i] == 0xFF && i+1 < len(b) {
					n := b[i+1]
					if n == 0x00 || (n >= 0xD0 && n <= 0xD7) || n == 0xFF {
						i += 2
						if n == 0xFF {
							i-- // fill byte: look at the next FF again
						}
						continue
					}
					break
				}
				i++
			}
			out.Write(b[start:i])
			inScan = false
			continue
		}
		if i+1 >= len(b) || b[i] != 0xFF {
			return nil, ErrImageFormat
		}
		marker := b[i+1]
		if marker == 0xFF { // fill byte before a marker
			i++
			continue
		}
		if marker == 0xD9 { // EOI: the end; whatever follows is dropped
			out.Write(b[i : i+2])
			if orientation > 1 {
				return withOrientation(out.Bytes(), orientation), nil
			}
			return out.Bytes(), nil
		}
		if marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 {
			out.Write(b[i : i+2])
			i += 2
			continue
		}
		if i+4 > len(b) {
			return nil, ErrImageFormat
		}
		size := int(binary.BigEndian.Uint16(b[i+2:]))
		end := i + 2 + size
		if size < 2 || end > len(b) {
			return nil, ErrImageFormat
		}
		seg, payload := b[i:end], b[i+4:end]
		switch {
		case marker == 0xE1: // EXIF or XMP: keep only the orientation, later
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) && orientation == 0 {
				orientation = exifOrientation(payload[6:])
			}
		case marker == 0xE2 && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")):
			out.Write(seg) // color profile
		case marker == 0xE0, marker == 0xEE: // JFIF; Adobe (needed for CMYK colors)
			out.Write(seg)
		case marker >= 0xE0 && marker <= 0xEF, marker == 0xFE:
			// Other APPn (IPTC, FlashPix, MPF, maker data) and comments: dropped.
		default:
			out.Write(seg)
		}
		i = end
		if marker == 0xDA {
			inScan = true
		}
	}
	return nil, ErrImageFormat // no EOI
}

// exifOrientation reads tag 0x0112 from IFD0 of a TIFF/EXIF block; 0 if none.
func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(t[4:8]))
	if off < 8 || off+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 {
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
		}
	}
	return 0
}

// withOrientation puts a minimal EXIF block holding only the orientation
// right after the JPEG's start (and its JFIF block, if any).
func withOrientation(j []byte, o int) []byte {
	exif := []byte("Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08" + // TIFF header, IFD0 at 8
		"\x00\x01" + // one entry
		"\x01\x12\x00\x03\x00\x00\x00\x01") // Orientation, SHORT, count 1
	exif = append(exif, 0, byte(o), 0, 0) // value, padded
	exif = append(exif, 0, 0, 0, 0)       // no next IFD
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(exif)+2))
	seg = append(seg, exif...)
	at := 2
	if len(j) > 6 && j[2] == 0xFF && j[3] == 0xE0 {
		at = 4 + int(binary.BigEndian.Uint16(j[4:]))
	}
	out := make([]byte, 0, len(j)+len(seg))
	out = append(out, j[:at]...)
	out = append(out, seg...)
	return append(out, j[at:]...)
}

// ---------------------------------------------------------------- PNG

// pngKeep are the chunks that make up the picture or its colors.
var pngKeep = map[string]bool{
	"IHDR": true, "PLTE": true, "IDAT": true, "IEND": true, "tRNS": true,
	"cHRM": true, "gAMA": true, "iCCP": true, "sBIT": true, "sRGB": true,
	"cICP": true, "mDCv": true, "cLLi": true, "bKGD": true,
	"acTL": true, "fcTL": true, "fdAT": true, // animation (APNG)
}

func stripPNG(b []byte) ([]byte, error) {
	out := bytes.NewBuffer(make([]byte, 0, len(b)))
	out.Write(b[:8])
	for i := 8; i+12 <= len(b); {
		size := int(binary.BigEndian.Uint32(b[i:]))
		end := i + 12 + size
		if size < 0 || end > len(b) {
			return nil, ErrImageFormat
		}
		kind := string(b[i+4 : i+8])
		if pngKeep[kind] {
			out.Write(b[i:end])
		}
		if kind == "IEND" {
			return out.Bytes(), nil
		}
		i = end
	}
	return nil, ErrImageFormat
}

// ---------------------------------------------------------------- WebP

func stripWebP(b []byte) ([]byte, error) {
	riff := int(binary.LittleEndian.Uint32(b[4:8]))
	if riff < 4 || 8+riff > len(b) {
		return nil, ErrImageFormat
	}
	body := bytes.NewBuffer(make([]byte, 0, riff))
	body.WriteString("WEBP")
	for i := 12; i < 8+riff; {
		if i+8 > len(b) {
			return nil, ErrImageFormat
		}
		kind := string(b[i : i+4])
		size := int(binary.LittleEndian.Uint32(b[i+4:]))
		end := i + 8 + size + size%2
		if end > 8+riff || end > len(b) {
			return nil, ErrImageFormat
		}
		switch kind {
		case "VP8X":
			c := append([]byte(nil), b[i:end]...)
			if len(c) > 8 {
				c[8] &^= 0x08 | 0x04 // no EXIF, no XMP
			}
			body.Write(c)
		case "VP8 ", "VP8L", "ALPH", "ANIM", "ANMF", "ICCP":
			body.Write(b[i:end])
		}
		i = end
	}
	out := make([]byte, 8, 8+body.Len())
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(body.Len()))
	return append(out, body.Bytes()...), nil
}

// ---------------------------------------------------------------- GIF

func stripGIF(b []byte) ([]byte, error) {
	if len(b) < 13 {
		return nil, ErrImageFormat
	}
	out := bytes.NewBuffer(make([]byte, 0, len(b)))
	i := 13
	if b[10]&0x80 != 0 {
		i += 3 << (int(b[10]&7) + 1)
	}
	if i > len(b) {
		return nil, ErrImageFormat
	}
	out.Write(b[:i])
	// subBlocks returns where a run of data sub-blocks ends.
	subBlocks := func(j int) (int, error) {
		for {
			if j >= len(b) {
				return 0, ErrImageFormat
			}
			n := int(b[j])
			j += 1 + n
			if n == 0 {
				return j, nil
			}
		}
	}
	for i < len(b) {
		switch b[i] {
		case 0x3B: // trailer: the end
			out.WriteByte(0x3B)
			return out.Bytes(), nil
		case 0x2C: // image
			j := i + 10
			if j > len(b) {
				return nil, ErrImageFormat
			}
			if b[i+9]&0x80 != 0 {
				j += 3 << (int(b[i+9]&7) + 1)
			}
			j++ // LZW minimum code size
			end, err := subBlocks(j)
			if err != nil {
				return nil, err
			}
			out.Write(b[i:end])
			i = end
		case 0x21: // extension
			if i+2 > len(b) {
				return nil, ErrImageFormat
			}
			end, err := subBlocks(i + 2)
			if err != nil {
				return nil, err
			}
			keep := false
			switch b[i+1] {
			case 0xF9, 0x01: // graphic control, plain text: part of the picture
				keep = true
			case 0xFF: // application: only looping and the color profile
				if i+14 <= len(b) && b[i+2] == 11 {
					app := string(b[i+3 : i+14])
					keep = app == "NETSCAPE2.0" || app == "ANIMEXTS1.0" || app == "ICCRGBG1012"
				}
			}
			if keep {
				out.Write(b[i:end])
			}
			i = end
		default:
			return nil, ErrImageFormat
		}
	}
	return nil, ErrImageFormat
}
