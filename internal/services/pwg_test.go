package services

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// decodePWG parses a PWG Raster stream back into per-page pixel grids, exercising
// the same header layout and RLE the encoder produces. It validates structural
// self-consistency and lets the test compare decoded pixels to the source.
func decodePWG(t *testing.T, data []byte) []*image.RGBA {
	t.Helper()
	if string(data[:4]) != "RaS2" {
		t.Fatalf("bad sync word: %q", data[:4])
	}
	r := bytes.NewReader(data[4:])

	var pages []*image.RGBA
	for r.Len() > 0 {
		hdr := make([]byte, pwgPageHeaderSize)
		if _, err := r.Read(hdr); err != nil {
			t.Fatalf("reading page header: %v", err)
		}
		if string(trimNUL(hdr[:64])) != "PwgRaster" {
			t.Fatalf("MediaClass = %q, want PwgRaster", trimNUL(hdr[:64]))
		}
		width := int(binary.BigEndian.Uint32(hdr[372:]))        // cupsWidth
		height := int(binary.BigEndian.Uint32(hdr[376:]))       // cupsHeight
		bitsPerPixel := int(binary.BigEndian.Uint32(hdr[388:])) // cupsBitsPerPixel
		bytesPerLine := int(binary.BigEndian.Uint32(hdr[392:])) // cupsBytesPerLine
		colorSpace := int(binary.BigEndian.Uint32(hdr[400:]))   // cupsColorSpace

		bpp := bitsPerPixel / 8
		if bpp != 3 {
			t.Fatalf("bytesPerPixel = %d, want 3", bpp)
		}
		if bytesPerLine != width*bpp {
			t.Fatalf("bytesPerLine = %d, want %d", bytesPerLine, width*bpp)
		}
		if colorSpace != cupsColorSpaceSRGB {
			t.Fatalf("colorSpace = %d, want %d", colorSpace, cupsColorSpaceSRGB)
		}

		img := image.NewRGBA(image.Rect(0, 0, width, height))
		y := 0
		for y < height {
			repeat, err := r.ReadByte()
			if err != nil {
				t.Fatalf("reading line repeat: %v", err)
			}
			line := decodePWGLine(t, r, bytesPerLine, bpp)
			for c := 0; c <= int(repeat) && y < height; c++ {
				for x := 0; x < width; x++ {
					o := x * bpp
					img.Set(x, y, color.RGBA{line[o], line[o+1], line[o+2], 255})
				}
				y++
			}
		}
		pages = append(pages, img)
	}
	return pages
}

func decodePWGLine(t *testing.T, r *bytes.Reader, bytesPerLine, bpp int) []byte {
	t.Helper()
	out := make([]byte, 0, bytesPerLine)
	for len(out) < bytesPerLine {
		count, err := r.ReadByte()
		if err != nil {
			t.Fatalf("reading run count: %v", err)
		}
		px := make([]byte, bpp)
		if count <= 127 { // run of count+1 identical pixels
			if _, err := r.Read(px); err != nil {
				t.Fatalf("reading run pixel: %v", err)
			}
			for i := 0; i <= int(count); i++ {
				out = append(out, px...)
			}
		} else { // literal of 257-count pixels
			num := 257 - int(count)
			for i := 0; i < num; i++ {
				if _, err := r.Read(px); err != nil {
					t.Fatalf("reading literal pixel: %v", err)
				}
				out = append(out, px...)
			}
		}
	}
	if len(out) != bytesPerLine {
		t.Fatalf("decoded line is %d bytes, want %d", len(out), bytesPerLine)
	}
	return out
}

func trimNUL(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return b[:i]
	}
	return b
}

// TestEncodePWGRoundTrip encodes a synthetic image with runs and literals, then
// decodes it and compares pixel-for-pixel.
func TestEncodePWGRoundTrip(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 3))
	// Row 0: a long run of red. Row 1: alternating (literals). Row 2: white.
	for x := 0; x < 8; x++ {
		src.Set(x, 0, color.RGBA{255, 0, 0, 255})
		if x%2 == 0 {
			src.Set(x, 1, color.RGBA{0, 0, 0, 255})
		} else {
			src.Set(x, 1, color.RGBA{0, 255, 0, 255})
		}
		src.Set(x, 2, color.RGBA{255, 255, 255, 255})
	}

	data, err := encodePWG([]image.Image{src}, 300)
	if err != nil {
		t.Fatalf("encodePWG: %v", err)
	}

	pages := decodePWG(t, data)
	if len(pages) != 1 {
		t.Fatalf("decoded %d pages, want 1", len(pages))
	}
	got := pages[0]
	if got.Bounds() != src.Bounds() {
		t.Fatalf("bounds %v, want %v", got.Bounds(), src.Bounds())
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 8; x++ {
			wr, wg, wb, _ := src.At(x, y).RGBA()
			gr, gg, gb, _ := got.At(x, y).RGBA()
			if wr != gr || wg != gg || wb != gb {
				t.Fatalf("pixel (%d,%d): got %v, want %v", x, y, got.At(x, y), src.At(x, y))
			}
		}
	}
}

// TestRenderArticleToImagesPWG renders a small article to bitmaps at a low DPI,
// confirms PWG encoding succeeds and decodes, and writes page 1 as a PNG for
// optional manual inspection (path logged).
func TestRenderArticleToImagesPWG(t *testing.T) {
	const html = `<h1>Section</h1>` +
		`<p>The quick brown fox jumps over the lazy dog. ` +
		`This paragraph wraps across the column to exercise layout.</p>` +
		`<ul><li>First item</li><li>Second item</li></ul>` +
		`<blockquote>A short quotation.</blockquote>`

	imgs, err := RenderArticleToImages("Round Trip Headline", "https://example.com", html, 120)
	if err != nil {
		t.Fatalf("RenderArticleToImages: %v", err)
	}
	if len(imgs) == 0 {
		t.Fatal("no pages rendered")
	}

	data, err := encodePWG(imgs, 120)
	if err != nil {
		t.Fatalf("encodePWG: %v", err)
	}
	if got := decodePWG(t, data); len(got) != len(imgs) {
		t.Fatalf("decoded %d pages, rendered %d", len(got), len(imgs))
	}
	t.Logf("rendered %d page(s), PWG stream %d bytes", len(imgs), len(data))

	out := filepath.Join(t.TempDir(), "page1.png")
	f, err := os.Create(out)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, imgs[0]); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	// Persist a copy under the repo's tmp dir when RSS_PRINT_DUMP is set, for
	// eyeballing the raster output.
	if dir := os.Getenv("RSS_PRINT_DUMP"); dir != "" {
		dst := filepath.Join(dir, "raster-page1.png")
		if b, rerr := os.ReadFile(out); rerr == nil {
			_ = os.WriteFile(dst, b, 0o644)
			t.Logf("dumped raster page to %s", dst)
		}
	}
}
