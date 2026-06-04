package services

import (
	"image/color"

	"github.com/signintech/gopdf"
)

// Font style flags. The values match gopdf's Regular/Italic/Bold/Underline
// constants so the gopdf backend can pass them through unchanged; the image
// backend maps them to the corresponding embedded face. styleUnderline is an
// independent bit that combines with the face flags (e.g. styleBold|styleUnderline).
const (
	styleRegular   = 0
	styleItalic    = 1
	styleBold      = 2
	styleUnderline = 4
)

// pageGeometry describes a page in PDF points (1/72 inch): physical size and
// content margins. The renderer's layout math is expressed entirely in terms of
// these fields so the same pass can target different paper sizes.
type pageGeometry struct {
	width, height                      float64
	marginL, marginR, marginT, marginB float64
}

func (g pageGeometry) columnWidth() float64 { return g.width - g.marginL - g.marginR }
func (g pageGeometry) bottom() float64      { return g.height - g.marginB }

// a4Geometry is the original layout used for browser-download PDFs.
var a4Geometry = pageGeometry{width: 595, height: 842, marginL: 56, marginR: 56, marginT: 64, marginB: 64}

// letterGeometry (US Letter, 8.5x11in) is used for raster output, since the
// common North American driverless printers default to na_letter media.
var letterGeometry = pageGeometry{width: 612, height: 792, marginL: 56, marginR: 56, marginT: 64, marginB: 64}

// canvas is the drawing surface the article renderer targets. Coordinates and
// sizes are in PDF points with the origin at the top-left of the page. Two
// backends implement it: gopdfCanvas (vector PDF) and imageCanvas (raster
// bitmap), letting one layout pass produce either output.
type canvas interface {
	// newPage starts a fresh page; subsequent draw calls target it.
	newPage()
	// measureText returns the rendered width of text at the given style and size.
	measureText(text string, style int, size float64) float64
	// textHeight returns the base line height for the given size (gopdf's "Hg"
	// cell height), before any leading multiplier.
	textHeight(size float64) float64
	// drawText draws a single line of text with its baseline box top-left at
	// (x, y).
	drawText(x, y float64, text string, style int, size float64, c color.Color)
	// drawLine strokes a line of the given width.
	drawLine(x1, y1, x2, y2, width float64, c color.Color)
	// drawImage decodes raw image bytes and draws them into the (w, h) box at
	// (x, y).
	drawImage(data []byte, x, y, w, h float64) error
}

// gopdfCanvas renders onto a gopdf document, reproducing the exact gopdf calls
// the renderer made before the canvas abstraction existed. PDF output is
// therefore unchanged.
type gopdfCanvas struct {
	pdf    *gopdf.GoPdf
	family string
}

func newGopdfCanvas(pdf *gopdf.GoPdf, family string) *gopdfCanvas {
	return &gopdfCanvas{pdf: pdf, family: family}
}

func (c *gopdfCanvas) newPage() { c.pdf.AddPage() }

// setStyle selects the font face, degrading to Regular when a styled face is not
// registered so a style switch never aborts rendering.
func (c *gopdfCanvas) setStyle(style int, size float64) {
	if err := c.pdf.SetFontWithStyle(c.family, style, size); err != nil {
		_ = c.pdf.SetFontWithStyle(c.family, styleRegular, size)
	}
}

func (c *gopdfCanvas) measureText(text string, style int, size float64) float64 {
	c.setStyle(style, size)
	w, err := c.pdf.MeasureTextWidth(text)
	if err != nil {
		return 0
	}
	return w
}

func (c *gopdfCanvas) textHeight(size float64) float64 {
	c.setStyle(styleRegular, size)
	h, err := c.pdf.MeasureCellHeightByText("Hg")
	if err != nil || h <= 0 {
		return size
	}
	return h
}

func (c *gopdfCanvas) drawText(x, y float64, text string, style int, size float64, col color.Color) {
	r, g, b := rgb8(col)
	c.pdf.SetTextColor(r, g, b)
	c.setStyle(style, size)
	c.pdf.SetXY(x, y)
	_ = c.pdf.Cell(nil, text)
}

func (c *gopdfCanvas) drawLine(x1, y1, x2, y2, width float64, col color.Color) {
	r, g, b := rgb8(col)
	c.pdf.SetLineWidth(width)
	c.pdf.SetStrokeColor(r, g, b)
	c.pdf.Line(x1, y1, x2, y2)
}

func (c *gopdfCanvas) drawImage(data []byte, x, y, w, h float64) error {
	holder, err := gopdf.ImageHolderByBytes(data)
	if err != nil {
		return err
	}
	return c.pdf.ImageByHolder(holder, x, y, &gopdf.Rect{W: w, H: h})
}

// rgb8 converts a color.Color to 8-bit RGB components.
func rgb8(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}
