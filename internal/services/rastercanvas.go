package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"rss-print/ui"
)

// imageCanvas renders the article layout to one RGBA bitmap per page. The
// renderer issues all calls in PDF points; this backend converts to device
// pixels with the scale factor dpi/72. Glyphs are rasterized at the device
// resolution (truetype faces baked with DPI=dpi), so text is crisp rather than a
// scaled-up low-res bitmap.
type imageCanvas struct {
	geom  pageGeometry
	dpi   float64
	scale float64 // device pixels per point
	wPx   int
	hPx   int

	regular *truetype.Font
	bold    *truetype.Font
	italic  *truetype.Font
	faces   map[faceKey]font.Face

	cur  *image.RGBA
	dc   *gg.Context // wraps cur for line/image drawing
	done []image.Image
}

type faceKey struct {
	style int
	size  float64
}

func newImageCanvas(geom pageGeometry, dpi float64, _ string) (*imageCanvas, error) {
	reg, err := parseEmbeddedFont("static/fonts/Roboto-Regular.ttf")
	if err != nil {
		return nil, err
	}
	bold, err := parseEmbeddedFont("static/fonts/Roboto-Bold.ttf")
	if err != nil {
		return nil, err
	}
	ital, err := parseEmbeddedFont("static/fonts/Roboto-Italic.ttf")
	if err != nil {
		return nil, err
	}

	scale := dpi / 72.0
	c := &imageCanvas{
		geom:    geom,
		dpi:     dpi,
		scale:   scale,
		wPx:     int(math.Round(geom.width * scale)),
		hPx:     int(math.Round(geom.height * scale)),
		regular: reg,
		bold:    bold,
		italic:  ital,
		faces:   make(map[faceKey]font.Face),
	}
	c.startPage()
	return c, nil
}

func parseEmbeddedFont(path string) (*truetype.Font, error) {
	b, err := ui.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded font %s: %w", path, err)
	}
	f, err := truetype.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font %s: %w", path, err)
	}
	return f, nil
}

// startPage allocates a fresh white page and the gg context that wraps it.
func (c *imageCanvas) startPage() {
	img := image.NewRGBA(image.Rect(0, 0, c.wPx, c.hPx))
	dc := gg.NewContextForRGBA(img)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	c.cur = img
	c.dc = dc
}

func (c *imageCanvas) newPage() {
	c.done = append(c.done, c.cur)
	c.startPage()
}

// pages returns every rendered page, including the one in progress.
func (c *imageCanvas) pages() []image.Image {
	return append(append([]image.Image(nil), c.done...), c.cur)
}

// fontFor resolves a style to one of the embedded faces. BoldItalic degrades to
// Bold, matching the PDF backend (no dedicated BoldItalic face is embedded).
func (c *imageCanvas) fontFor(style int) *truetype.Font {
	switch {
	case style&styleBold != 0:
		return c.bold
	case style&styleItalic != 0:
		return c.italic
	default:
		return c.regular
	}
}

func (c *imageCanvas) face(style int, size float64) font.Face {
	key := faceKey{style: style, size: size}
	if f, ok := c.faces[key]; ok {
		return f
	}
	f := truetype.NewFace(c.fontFor(style), &truetype.Options{
		Size:    size, // points; DPI scales to device pixels
		DPI:     c.dpi,
		Hinting: font.HintingFull,
	})
	c.faces[key] = f
	return f
}

func (c *imageCanvas) measureText(text string, style int, size float64) float64 {
	adv := font.MeasureString(c.face(style, size), text)
	return float64(adv) / 64.0 / c.scale // device px -> points
}

func (c *imageCanvas) textHeight(size float64) float64 {
	m := c.face(styleRegular, size).Metrics()
	return float64(m.Height) / 64.0 / c.scale
}

func (c *imageCanvas) drawText(x, y float64, text string, style int, size float64, col color.Color) {
	face := c.face(style, size)
	ascentPx := float64(face.Metrics().Ascent) / 64.0
	d := &font.Drawer{
		Dst:  c.cur,
		Src:  image.NewUniform(col),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.Int26_6(math.Round(x * c.scale * 64)),
			Y: fixed.Int26_6(math.Round((y*c.scale + ascentPx) * 64)),
		},
	}
	d.DrawString(text)
}

func (c *imageCanvas) drawLine(x1, y1, x2, y2, width float64, col color.Color) {
	c.dc.SetColor(col)
	c.dc.SetLineWidth(width * c.scale)
	c.dc.DrawLine(x1*c.scale, y1*c.scale, x2*c.scale, y2*c.scale)
	c.dc.Stroke()
}

func (c *imageCanvas) drawImage(data []byte, x, y, w, h float64) error {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	dstRect := image.Rect(
		int(math.Round(x*c.scale)),
		int(math.Round(y*c.scale)),
		int(math.Round((x+w)*c.scale)),
		int(math.Round((y+h)*c.scale)),
	)
	draw.CatmullRom.Scale(c.cur, dstRect, src, src.Bounds(), draw.Over, nil)
	return nil
}
