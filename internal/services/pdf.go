package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"strings"

	"rss-print/ui"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
)

const pdfFontFamily = "Roboto"

// ArticleDocument bundles a rendered article in the forms the printer pipeline
// needs: the cleaned source HTML (used to rasterize for raster-only printers) and
// the prebuilt PDF (used for PDF-capable printers and the browser download).
type ArticleDocument struct {
	Title     string
	BaseURL   string
	CleanHTML string
	PDF       []byte
}

// BuildArticleDocument resolves an article's cleaned HTML (using stored content
// when available, otherwise extracting on demand and backfilling via persist),
// renders the PDF, and returns both forms. When no content can be obtained it
// synthesizes a minimal document referencing the source URL so the raster path
// always has HTML to render.
func BuildArticleDocument(ctx context.Context, title, url, storedContent string, persist func(content string)) (*ArticleDocument, error) {
	content := storedContent
	if strings.TrimSpace(content) == "" {
		clean, _, err := ExtractArticleHTML(ctx, url, "")
		if err == nil && strings.TrimSpace(clean) != "" {
			content = clean
			if persist != nil {
				persist(content)
			}
		}
	}
	if strings.TrimSpace(content) == "" {
		content = "<p>Original URL: " + html.EscapeString(url) + "</p>"
	}

	pdfBytes, err := generateArticlePDFFromHTML(title, url, content)
	if err != nil {
		return nil, err
	}
	return &ArticleDocument{Title: title, BaseURL: url, CleanHTML: content, PDF: pdfBytes}, nil
}

// BuildArticlePDF renders an article PDF using stored content when available.
// When storedContent is empty, it extracts the article on demand and, if persist
// is non-nil, hands the cleaned HTML back so the caller can backfill the row.
func BuildArticlePDF(ctx context.Context, title, url, storedContent string, persist func(content string)) ([]byte, error) {
	doc, err := BuildArticleDocument(ctx, title, url, storedContent, persist)
	if err != nil {
		return nil, err
	}
	return doc.PDF, nil
}

// generateArticlePDFFromHTML renders a newspaper-style PDF from already-cleaned
// article HTML. baseURL resolves relative image/link references; pass the article
// URL when known.
func generateArticlePDFFromHTML(title, baseURL, cleanHTML string) ([]byte, error) {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	if err := loadFonts(&pdf); err != nil {
		return nil, err
	}

	pdf.AddPage()

	cv := newGopdfCanvas(&pdf, pdfFontFamily)
	r := newPDFRenderer(cv, a4Geometry, baseURL)
	if err := r.renderDocument(title, cleanHTML); err != nil {
		return nil, fmt.Errorf("failed to render article: %w", err)
	}

	var b bytes.Buffer
	if _, err := pdf.WriteTo(&b); err != nil {
		return nil, fmt.Errorf("failed to generate pdf buffer: %w", err)
	}
	return b.Bytes(), nil
}

// renderArticleToImages renders cleaned article HTML to one bitmap per page at the
// given resolution, for encoding to a printer-native raster format. Pages use US
// Letter geometry to match the common na_letter driverless default.
func renderArticleToImages(title, baseURL, cleanHTML string, dpi float64) ([]image.Image, error) {
	cv, err := newImageCanvas(letterGeometry, dpi, pdfFontFamily)
	if err != nil {
		return nil, err
	}

	r := newPDFRenderer(cv, letterGeometry, baseURL)
	if err := r.renderDocument(title, cleanHTML); err != nil {
		return nil, fmt.Errorf("failed to render article to raster: %w", err)
	}
	return cv.pages(), nil
}

// loadFonts registers the Roboto family faces (Regular, Bold, Italic) from the
// embedded UI filesystem. Bold and italic each require a separately registered
// TTF face; gopdf cannot synthesize them. BoldItalic degrades to Bold when the
// dedicated face is absent.
func loadFonts(pdf *gopdf.GoPdf) error {
	faces := []struct {
		file  string
		style int
	}{
		{"static/fonts/Roboto-Regular.ttf", gopdf.Regular},
		{"static/fonts/Roboto-Bold.ttf", gopdf.Bold},
		{"static/fonts/Roboto-Italic.ttf", gopdf.Italic},
	}

	for _, f := range faces {
		fontBytes, err := ui.FS.ReadFile(f.file)
		if err != nil {
			return fmt.Errorf("failed to read embedded font %s: %w", f.file, err)
		}
		err = pdf.AddTTFFontByReaderWithOption(pdfFontFamily, bytes.NewReader(fontBytes), gopdf.TtfOption{Style: f.style})
		if err != nil {
			return fmt.Errorf("failed to add font %s: %w", f.file, err)
		}
	}

	// Map a BoldItalic request to the Bold face so style switches never error.
	if boldBytes, err := ui.FS.ReadFile("static/fonts/Roboto-Bold.ttf"); err == nil {
		_ = pdf.AddTTFFontByReaderWithOption(pdfFontFamily, bytes.NewReader(boldBytes), gopdf.TtfOption{Style: gopdf.Bold | gopdf.Italic})
	}

	return nil
}
