package services

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for image.DecodeConfig
	_ "image/jpeg" // register JPEG decoder for image.DecodeConfig
	_ "image/png"  // register PNG decoder for image.DecodeConfig
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

// A4 page geometry and margins, in PDF points (gopdf's default unit).
const (
	pdfPageWidth   = 595.0
	pdfPageHeight  = 842.0
	pdfMarginL     = 56.0
	pdfMarginR     = 56.0
	pdfMarginT     = 64.0
	pdfMarginB     = 64.0
	pdfColumnWidth = pdfPageWidth - pdfMarginL - pdfMarginR
	pdfPageBottom  = pdfPageHeight - pdfMarginB

	pdfUserAgent  = "rss-print/1.0 (+https://github.com/cole-maxwell1/rss-print)"
	maxImageBytes = 8 << 20 // 8 MB cap per image
)

// pdfRenderer walks cleaned article HTML and emits a paginated, newspaper-style
// layout into a gopdf document. It owns the cursor and performs manual
// pagination, since gopdf provides no automatic page breaks.
type pdfRenderer struct {
	pdf    *gopdf.GoPdf
	family string
	base   *url.URL

	x, y       float64
	page       int
	heightByPt map[float64]float64 // cached base line height per font size
	httpc      *http.Client
}

// run is a contiguous span of inline text sharing one font style.
type run struct {
	text  string
	style int
}

// word is a single whitespace-delimited token carrying its style, or the
// sentinel "\n" marking a forced line break (from <br>).
type word struct {
	text  string
	style int
}

func newPDFRenderer(pdf *gopdf.GoPdf, family, baseURL string) *pdfRenderer {
	var base *url.URL
	if u, err := url.Parse(strings.TrimSpace(baseURL)); err == nil && u.IsAbs() {
		base = u
	}
	return &pdfRenderer{
		pdf:        pdf,
		family:     family,
		base:       base,
		x:          pdfMarginL,
		y:          pdfMarginT,
		page:       1,
		heightByPt: make(map[float64]float64),
		httpc:      &http.Client{Timeout: 15 * time.Second},
	}
}

// renderDocument writes the title masthead followed by the article body.
func (r *pdfRenderer) renderDocument(title, cleanHTML string) error {
	r.writeWords([]word{{text: title, style: gopdf.Bold}}, 24, 0, 1.2)
	r.y += 8
	r.ensureSpace(2)
	r.pdf.SetLineWidth(1)
	r.pdf.SetStrokeColor(0, 0, 0)
	r.pdf.Line(pdfMarginL, r.y, pdfMarginL+pdfColumnWidth, r.y)
	r.y += 16

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(cleanHTML))
	if err != nil {
		return fmt.Errorf("failed to parse article html: %w", err)
	}

	body := doc.Find("body")
	if body.Length() == 0 {
		body = doc.Selection
	}
	body.Children().Each(func(_ int, s *goquery.Selection) {
		r.renderBlock(s)
	})
	return nil
}

// renderBlock dispatches a block-level element to the matching handler.
func (r *pdfRenderer) renderBlock(s *goquery.Selection) {
	if len(s.Nodes) == 0 || s.Nodes[0].Type != html.ElementNode {
		return
	}
	switch s.Nodes[0].Data {
	case "h1":
		r.renderHeading(s, 1)
	case "h2":
		r.renderHeading(s, 2)
	case "h3":
		r.renderHeading(s, 3)
	case "h4", "h5", "h6":
		r.renderHeading(s, 4)
	case "p", "pre":
		r.renderParagraph(s)
	case "ul":
		r.renderList(s, false, 0)
	case "ol":
		r.renderList(s, true, 0)
	case "blockquote":
		r.renderBlockquote(s)
	case "figure":
		r.renderFigure(s)
	case "img":
		r.renderImage(s.AttrOr("src", ""), "")
	case "hr":
		r.renderRule()
	default:
		// Containers (div/section/article/...) carry their content in children.
		s.Children().Each(func(_ int, c *goquery.Selection) {
			r.renderBlock(c)
		})
	}
}

func (r *pdfRenderer) renderHeading(s *goquery.Selection, level int) {
	size := map[int]float64{1: 22, 2: 18, 3: 15, 4: 13}[level]
	r.y += 8
	r.writeWords(runsToWords(r.inlineRuns(s, gopdf.Bold)), size, 0, 1.2)
	r.y += 6
}

func (r *pdfRenderer) renderParagraph(s *goquery.Selection) {
	words := runsToWords(r.inlineRuns(s, gopdf.Regular))
	if len(words) == 0 {
		return
	}
	r.writeWords(words, 11, 0, 1.4)
	r.y += 7
}

func (r *pdfRenderer) renderList(s *goquery.Selection, ordered bool, depth int) {
	indent := 18.0 * float64(depth+1)
	item := 0
	s.Children().Each(func(_ int, li *goquery.Selection) {
		if len(li.Nodes) == 0 || li.Nodes[0].Data != "li" {
			return
		}
		item++
		marker := "• "
		if ordered {
			marker = fmt.Sprintf("%d. ", item)
		}
		runs := append([]run{{text: marker, style: gopdf.Regular}}, r.inlineRuns(li, gopdf.Regular)...)
		words := runsToWords(runs)
		if len(words) > 0 {
			r.writeWords(words, 11, indent, 1.35)
			r.y += 3
		}
		// Nested lists render one level deeper.
		li.Children().Each(func(_ int, c *goquery.Selection) {
			switch {
			case c.Is("ul"):
				r.renderList(c, false, depth+1)
			case c.Is("ol"):
				r.renderList(c, true, depth+1)
			}
		})
	})
	r.y += 4
}

func (r *pdfRenderer) renderBlockquote(s *goquery.Selection) {
	const indent = 24.0
	startY := r.y
	startPage := r.page
	r.y += 2

	rendered := false
	s.Children().Each(func(_ int, c *goquery.Selection) {
		if len(c.Nodes) == 0 || c.Nodes[0].Type != html.ElementNode || !isBlockTag(c.Nodes[0].Data) {
			return
		}
		words := runsToWords(r.inlineRuns(c, gopdf.Italic))
		if len(words) > 0 {
			r.writeWords(words, 11, indent, 1.4)
			r.y += 5
			rendered = true
		}
	})
	if !rendered {
		words := runsToWords(r.inlineRuns(s, gopdf.Italic))
		if len(words) > 0 {
			r.writeWords(words, 11, indent, 1.4)
			r.y += 5
		}
	}

	// Draw the quote rule only when the block stayed on a single page; spanning
	// a page break is a known v1 limitation.
	if r.page == startPage && r.y-5 > startY {
		r.pdf.SetLineWidth(2)
		r.pdf.SetStrokeColor(180, 180, 180)
		r.pdf.Line(pdfMarginL+8, startY, pdfMarginL+8, r.y-5)
		r.pdf.SetStrokeColor(0, 0, 0)
	}
	r.y += 4
}

func (r *pdfRenderer) renderFigure(s *goquery.Selection) {
	img := s.Find("img").First()
	caption := strings.TrimSpace(s.Find("figcaption").First().Text())
	r.renderImage(img.AttrOr("src", ""), caption)
}

func (r *pdfRenderer) renderRule() {
	r.y += 6
	r.ensureSpace(2)
	r.pdf.SetLineWidth(0.5)
	r.pdf.SetStrokeColor(200, 200, 200)
	r.pdf.Line(pdfMarginL, r.y, pdfMarginL+pdfColumnWidth, r.y)
	r.pdf.SetStrokeColor(0, 0, 0)
	r.y += 8
}

// renderImage downloads, scales, and places an image, skipping silently on any
// failure or unsupported format so one bad image never aborts the document.
func (r *pdfRenderer) renderImage(src, caption string) {
	data, _, ok := r.downloadImage(src)
	if !ok {
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return
	}

	iw, ih := float64(cfg.Width), float64(cfg.Height)
	dispW := pdfColumnWidth
	if iw < dispW {
		dispW = iw
	}
	dispH := ih * (dispW / iw)
	if maxH := pdfPageBottom - pdfMarginT; dispH > maxH {
		dispH = maxH
		dispW = iw * (dispH / ih)
	}

	holder, err := gopdf.ImageHolderByBytes(data)
	if err != nil {
		return
	}

	r.y += 4
	r.ensureSpace(dispH)
	x := pdfMarginL + (pdfColumnWidth-dispW)/2
	if err := r.pdf.ImageByHolder(holder, x, r.y, &gopdf.Rect{W: dispW, H: dispH}); err != nil {
		log.Printf("pdf: failed to place image %s: %v", src, err)
		return
	}
	r.y += dispH + 4

	if caption != "" {
		r.pdf.SetTextColor(120, 120, 120)
		r.writeWords([]word{{text: caption, style: gopdf.Italic}}, 9, 12, 1.3)
		r.pdf.SetTextColor(0, 0, 0)
		r.y += 6
	}
}

// downloadImage fetches an image with a timeout and size cap, returning ok=false
// for any error or for formats gopdf cannot embed (only jpeg/png/gif pass).
func (r *pdfRenderer) downloadImage(rawURL string) ([]byte, string, bool) {
	u := r.resolveURL(rawURL)
	if u == "" {
		return nil, "", false
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, "", false
	}
	req.Header.Set("User-Agent", pdfUserAgent)

	resp, err := r.httpc.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", false
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		return nil, "", false
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil || len(data) > maxImageBytes {
		return nil, "", false
	}

	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", false
	}
	switch format {
	case "jpeg", "png", "gif":
		return data, format, true
	default:
		return nil, "", false
	}
}

func (r *pdfRenderer) resolveURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		return u.String()
	}
	if r.base != nil {
		return r.base.ResolveReference(u).String()
	}
	return ""
}

// inlineRuns flattens an element's inline content into style-tagged runs,
// honoring nested <strong>/<em>/<a>/<br>. Nested block-level elements are
// skipped; they are handled separately by the block dispatcher.
func (r *pdfRenderer) inlineRuns(s *goquery.Selection, style int) []run {
	var runs []run
	s.Contents().Each(func(_ int, c *goquery.Selection) {
		node := c.Nodes[0]
		switch node.Type {
		case html.TextNode:
			if strings.TrimSpace(node.Data) != "" {
				runs = append(runs, run{text: node.Data, style: style})
			}
		case html.ElementNode:
			switch node.Data {
			case "strong", "b":
				runs = append(runs, r.inlineRuns(c, style|gopdf.Bold)...)
			case "em", "i", "cite":
				runs = append(runs, r.inlineRuns(c, style|gopdf.Italic)...)
			case "br":
				runs = append(runs, run{text: "\n", style: style})
			default:
				if !isBlockTag(node.Data) {
					runs = append(runs, r.inlineRuns(c, style)...)
				}
			}
		}
	})
	return runs
}

// writeWords renders style-segmented words with greedy word-wrap at the given
// font size and left indent, page-breaking per line. leading scales the base
// line height.
func (r *pdfRenderer) writeWords(words []word, size, indent, leading float64) {
	if len(words) == 0 {
		return
	}
	maxW := pdfColumnWidth - indent
	lh := r.lineHeight(size, leading)
	spaceW := r.measure(" ", gopdf.Regular, size)

	var line []word
	var lineW float64

	flush := func() {
		if len(line) == 0 {
			return
		}
		r.ensureSpace(lh)
		x := pdfMarginL + indent
		for i, w := range line {
			text := w.text
			if i < len(line)-1 {
				text += " "
			}
			r.setStyle(w.style, size)
			r.pdf.SetXY(x, r.y)
			r.pdf.Cell(nil, text)
			x += r.measure(text, w.style, size)
		}
		r.y += lh
		line = nil
		lineW = 0
	}

	for _, w := range words {
		if w.text == "\n" {
			flush()
			continue
		}
		ww := r.measure(w.text, w.style, size)
		add := ww
		if len(line) > 0 {
			add += spaceW
		}
		if lineW+add > maxW && len(line) > 0 {
			flush()
		}
		if lineW > 0 {
			lineW += spaceW
		}
		lineW += ww
		line = append(line, w)
	}
	flush()
}

func (r *pdfRenderer) measure(text string, style int, size float64) float64 {
	r.setStyle(style, size)
	w, err := r.pdf.MeasureTextWidth(text)
	if err != nil {
		return 0
	}
	return w
}

// setStyle selects the font face for the given style, degrading to Regular when
// a face is not registered so a style switch never aborts rendering.
func (r *pdfRenderer) setStyle(style int, size float64) {
	if err := r.pdf.SetFontWithStyle(r.family, style, size); err != nil {
		_ = r.pdf.SetFontWithStyle(r.family, gopdf.Regular, size)
	}
}

func (r *pdfRenderer) lineHeight(size, leading float64) float64 {
	base, ok := r.heightByPt[size]
	if !ok {
		r.setStyle(gopdf.Regular, size)
		h, err := r.pdf.MeasureCellHeightByText("Hg")
		if err != nil || h <= 0 {
			h = size
		}
		base = h
		r.heightByPt[size] = base
	}
	return base * leading
}

func (r *pdfRenderer) ensureSpace(h float64) {
	if r.y+h > pdfPageBottom {
		r.newPage()
	}
}

func (r *pdfRenderer) newPage() {
	r.pdf.AddPage()
	r.page++
	r.x = pdfMarginL
	r.y = pdfMarginT
}

// runsToWords splits runs into whitespace-delimited words, preserving the "\n"
// forced-break sentinel produced by <br>.
func runsToWords(runs []run) []word {
	var words []word
	for _, rn := range runs {
		if rn.text == "\n" {
			words = append(words, word{text: "\n", style: rn.style})
			continue
		}
		for _, f := range strings.Fields(rn.text) {
			words = append(words, word{text: f, style: rn.style})
		}
	}
	return words
}

func isBlockTag(tag string) bool {
	switch tag {
	case "p", "div", "section", "article", "header", "footer", "aside",
		"ul", "ol", "li", "blockquote", "figure", "figcaption",
		"h1", "h2", "h3", "h4", "h5", "h6", "pre", "table", "hr":
		return true
	}
	return false
}
