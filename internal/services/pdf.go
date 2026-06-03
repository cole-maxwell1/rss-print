package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
	"rss-print/ui"
)

const pdfFontFamily = "Roboto"

// GenerateArticlePDF builds the PDF for an article from its URL. It fetches and
// extracts the readable article content, then renders a newspaper-style layout.
// Prefer GenerateArticlePDFFromHTML when cleaned content is already stored.
func GenerateArticlePDF(title, url string) ([]byte, error) {
	cleanHTML, baseURL, err := ExtractArticleHTML(context.Background(), url, "")
	if err != nil || strings.TrimSpace(cleanHTML) == "" {
		// Fall back to a minimal document referencing the source.
		return GeneratePDF(title, "Original URL: "+url)
	}
	return GenerateArticlePDFFromHTML(title, baseURL, cleanHTML)
}

// BuildArticlePDF renders an article PDF using stored content when available.
// When storedContent is empty, it extracts the article on demand and, if persist
// is non-nil, hands the cleaned HTML back so the caller can backfill the row.
// Falls back to a minimal document when no content can be obtained.
func BuildArticlePDF(ctx context.Context, title, url, storedContent string, persist func(content string)) ([]byte, error) {
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
		return GeneratePDF(title, "Original URL: "+url)
	}
	return GenerateArticlePDFFromHTML(title, url, content)
}

// GenerateArticlePDFFromHTML renders a newspaper-style PDF from already-cleaned
// article HTML. baseURL resolves relative image/link references; pass the article
// URL when known.
func GenerateArticlePDFFromHTML(title, baseURL, cleanHTML string) ([]byte, error) {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	if err := loadFonts(&pdf); err != nil {
		return nil, err
	}

	pdf.AddPage()

	r := newPDFRenderer(&pdf, pdfFontFamily, baseURL)
	if err := r.renderDocument(title, cleanHTML); err != nil {
		return nil, fmt.Errorf("failed to render article: %w", err)
	}

	var b bytes.Buffer
	if _, err := pdf.WriteTo(&b); err != nil {
		return nil, fmt.Errorf("failed to generate pdf buffer: %w", err)
	}
	return b.Bytes(), nil
}

// GeneratePDF creates a simple plain-text PDF document. Retained as a fallback
// for content that is not HTML or when extraction fails.
func GeneratePDF(title, content string) ([]byte, error) {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	if err := loadFonts(&pdf); err != nil {
		return nil, err
	}

	pdf.AddPage()

	if err := pdf.SetFontWithStyle(pdfFontFamily, gopdf.Bold, 20); err != nil {
		return nil, err
	}
	pdf.SetXY(pdfMarginL, pdfMarginT)
	pdf.Cell(nil, title)
	pdf.Br(28)

	if err := pdf.SetFontWithStyle(pdfFontFamily, gopdf.Regular, 12); err != nil {
		return nil, err
	}
	pdf.SetX(pdfMarginL)

	cleanText := stripHTML(content)
	if err := pdf.MultiCell(&gopdf.Rect{W: pdfColumnWidth, H: 800}, cleanText); err != nil {
		return nil, fmt.Errorf("failed to write content: %w", err)
	}

	var b bytes.Buffer
	if _, err := pdf.WriteTo(&b); err != nil {
		return nil, fmt.Errorf("failed to generate pdf buffer: %w", err)
	}
	return b.Bytes(), nil
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

func stripHTML(htmlStr string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(htmlStr))
	var text strings.Builder

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			if tokenizer.Err() == io.EOF {
				return text.String()
			}
			return htmlStr // fallback
		}

		if tt == html.TextToken {
			text.WriteString(string(tokenizer.Text()))
		}
	}
}
