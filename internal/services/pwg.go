package services

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"math"
)

// PWG Raster color space codes (from the CUPS raster definitions PWG Raster
// reuses). Only the two this project emits are defined.
const (
	cupsColorSpaceSGray = 18 // sgray_8
	cupsColorSpaceSRGB  = 19 // srgb_8
)

// pwgPageHeaderSize is the fixed size of a PWG Raster (cupsPageHeader2) page
// header, per PWG 5102.4.
const pwgPageHeaderSize = 1796

// encodePWG encodes one or more page bitmaps into a single PWG Raster document
// (image/pwg-raster). Pages are emitted as 8-bit sRGB; the stream begins with the
// big-endian "RaS2" sync word followed by a header + RLE-compressed data per page.
func encodePWG(images []image.Image, dpi float64) ([]byte, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("no pages to encode as PWG raster")
	}
	var buf bytes.Buffer
	buf.WriteString("RaS2") // big-endian PWG Raster v2 sync word
	for i, img := range images {
		if err := writePWGPage(&buf, img, dpi); err != nil {
			return nil, fmt.Errorf("encoding PWG page %d: %w", i+1, err)
		}
	}
	return buf.Bytes(), nil
}

// writePWGPage writes one page: a 1796-byte header followed by RLE raster data.
func writePWGPage(buf *bytes.Buffer, img image.Image, dpi float64) error {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("empty page bounds")
	}

	const numColors = 3 // sRGB chunky
	res := uint32(math.Round(dpi))
	widthPt := uint32(math.Round(float64(w) / dpi * 72))
	heightPt := uint32(math.Round(float64(h) / dpi * 72))

	hdr := newPWGHeader()
	hdr.str("PwgRaster")           // MediaClass
	hdr.str("")                    // MediaColor
	hdr.str("")                    // MediaType
	hdr.str("")                    // OutputType
	hdr.u32(0)                     // AdvanceDistance
	hdr.u32(0)                     // AdvanceMedia
	hdr.u32(0)                     // Collate
	hdr.u32(0)                     // CutMedia
	hdr.u32(0)                     // Duplex
	hdr.u32(res)                   // HWResolution[0]
	hdr.u32(res)                   // HWResolution[1]
	hdr.u32(0)                     // ImagingBoundingBox[0]
	hdr.u32(0)                     // ImagingBoundingBox[1]
	hdr.u32(widthPt)               // ImagingBoundingBox[2]
	hdr.u32(heightPt)              // ImagingBoundingBox[3]
	hdr.u32(0)                     // InsertSheet
	hdr.u32(0)                     // Jog
	hdr.u32(0)                     // LeadingEdge
	hdr.u32(0)                     // Margins[0]
	hdr.u32(0)                     // Margins[1]
	hdr.u32(0)                     // ManualFeed
	hdr.u32(0)                     // MediaPosition
	hdr.u32(0)                     // MediaWeight
	hdr.u32(0)                     // MirrorPrint
	hdr.u32(0)                     // NegativePrint
	hdr.u32(1)                     // NumCopies
	hdr.u32(0)                     // Orientation
	hdr.u32(0)                     // OutputFaceUp
	hdr.u32(widthPt)               // PageSize[0]
	hdr.u32(heightPt)              // PageSize[1]
	hdr.u32(0)                     // Separations
	hdr.u32(0)                     // TraySwitch
	hdr.u32(0)                     // Tumble
	hdr.u32(uint32(w))             // cupsWidth
	hdr.u32(uint32(h))             // cupsHeight
	hdr.u32(0)                     // cupsMediaType
	hdr.u32(8)                     // cupsBitsPerColor
	hdr.u32(8 * numColors)         // cupsBitsPerPixel
	hdr.u32(uint32(w * numColors)) // cupsBytesPerLine
	hdr.u32(0)                     // cupsColorOrder (chunky)
	hdr.u32(cupsColorSpaceSRGB)    // cupsColorSpace
	hdr.u32(0)                     // cupsCompression
	hdr.u32(0)                     // cupsRowCount
	hdr.u32(0)                     // cupsRowFeed
	hdr.u32(0)                     // cupsRowStep
	hdr.u32(numColors)             // cupsNumColors
	hdr.f32(1)                     // cupsBorderlessScalingFactor
	hdr.f32(float32(widthPt))      // cupsPageSize[0]
	hdr.f32(float32(heightPt))     // cupsPageSize[1]
	hdr.f32(0)                     // cupsImagingBBox[0]
	hdr.f32(0)                     // cupsImagingBBox[1]
	hdr.f32(float32(widthPt))      // cupsImagingBBox[2]
	hdr.f32(float32(heightPt))     // cupsImagingBBox[3]
	hdr.skip(16 * 4)               // cupsInteger[16]
	hdr.skip(16 * 4)               // cupsReal[16]
	hdr.skip(16 * 64)              // cupsString[16][64]
	hdr.str("")                    // cupsMarkerType
	hdr.str("")                    // cupsRenderingIntent
	hdr.str("na_letter_8.5x11in")  // cupsPageSizeName
	if hdr.off != pwgPageHeaderSize {
		return fmt.Errorf("PWG header is %d bytes, expected %d", hdr.off, pwgPageHeaderSize)
	}
	buf.Write(hdr.b)

	// Raster data: per-line PWG RLE, with identical consecutive lines coalesced
	// via the leading line-repeat byte (value = repeats beyond the first).
	var prevLine, prevEnc []byte
	repeat := 0
	flush := func() {
		buf.WriteByte(byte(repeat))
		buf.Write(prevEnc)
	}
	for y := range h {
		line := extractRGB(img, b, y, w)
		if prevLine != nil && repeat < 255 && bytes.Equal(line, prevLine) {
			repeat++
			continue
		}
		if prevLine != nil {
			flush()
		}
		prevLine = line
		prevEnc = encodePWGLine(line, numColors)
		repeat = 0
	}
	if prevLine != nil {
		flush()
	}
	return nil
}

// extractRGB returns row y as packed RGB bytes (alpha dropped). The page is fully
// opaque, so dropping alpha is lossless.
func extractRGB(img image.Image, bounds image.Rectangle, y, w int) []byte {
	out := make([]byte, w*3)
	if rgba, ok := img.(*image.RGBA); ok {
		off := rgba.PixOffset(bounds.Min.X, bounds.Min.Y+y)
		row := rgba.Pix[off : off+w*4]
		for x := range w {
			o := x * 4
			out[x*3] = row[o]
			out[x*3+1] = row[o+1]
			out[x*3+2] = row[o+2]
		}
		return out
	}
	for x := range w {
		r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
		out[x*3] = byte(r >> 8)
		out[x*3+1] = byte(g >> 8)
		out[x*3+2] = byte(b >> 8)
	}
	return out
}

// encodePWGLine PackBits-encodes one raster line. bpp is bytes per pixel. A count
// byte 0..127 means the following pixel repeats (count+1) times; 129..255 means
// (257-count) literal pixels follow.
func encodePWGLine(pixels []byte, bpp int) []byte {
	var out bytes.Buffer
	n := len(pixels) / bpp
	i := 0
	for i < n {
		// Length of the run of identical pixels starting at i (capped at 128).
		runEnd := i + 1
		for runEnd < n && runEnd-i < 128 && samePixel(pixels, i, runEnd, bpp) {
			runEnd++
		}
		if runEnd-i >= 2 {
			out.WriteByte(byte(runEnd - i - 1)) // 0..127
			out.Write(pixels[i*bpp : (i+1)*bpp])
			i = runEnd
			continue
		}

		// Literal run: distinct pixels until a >=2 run begins or the cap is hit.
		litEnd := i
		for litEnd < n && litEnd-i < 128 {
			if litEnd+1 < n && samePixel(pixels, litEnd, litEnd+1, bpp) {
				break
			}
			litEnd++
		}
		if litEnd-i == 1 {
			out.WriteByte(0) // single pixel encoded as a run of length 1
			out.Write(pixels[i*bpp : (i+1)*bpp])
		} else {
			out.WriteByte(byte(257 - (litEnd - i))) // 129..255
			out.Write(pixels[i*bpp : litEnd*bpp])
		}
		i = litEnd
	}
	return out.Bytes()
}

func samePixel(pixels []byte, a, b, bpp int) bool {
	return bytes.Equal(pixels[a*bpp:a*bpp+bpp], pixels[b*bpp:b*bpp+bpp])
}

// pwgHeader builds the fixed-size page header field by field via a moving cursor,
// keeping the byte layout self-checking against pwgPageHeaderSize.
type pwgHeader struct {
	b   []byte
	off int
}

func newPWGHeader() *pwgHeader { return &pwgHeader{b: make([]byte, pwgPageHeaderSize)} }

func (h *pwgHeader) str(s string) {
	copy(h.b[h.off:h.off+64], s) // remainder stays zero (null-padded)
	h.off += 64
}

func (h *pwgHeader) u32(v uint32) {
	binary.BigEndian.PutUint32(h.b[h.off:], v)
	h.off += 4
}

func (h *pwgHeader) f32(v float32) {
	binary.BigEndian.PutUint32(h.b[h.off:], math.Float32bits(v))
	h.off += 4
}

func (h *pwgHeader) skip(n int) { h.off += n }

// encodeJPEG encodes the first page as a JPEG (image/jpeg). It is a last-resort
// fallback for printers that accept neither PDF nor PWG raster; JPEG carries a
// single page, so additional pages are dropped (and logged).
func encodeJPEG(images []image.Image) ([]byte, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("no pages to encode as JPEG")
	}
	if len(images) > 1 {
		log.Printf("encodeJPEG: article has %d pages; JPEG fallback sends only the first", len(images))
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, images[0], &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
