package services

import (
	"os"
	"strings"
	"testing"
)

// TestRenderNewspaperPDF exercises the HTML-to-gopdf renderer offline: inline
// bold/italic runs, headings, lists, a blockquote, and manual pagination across
// several pages. Set PDF_OUT to dump the result for visual inspection.
func TestRenderNewspaperPDF(t *testing.T) {
	var body strings.Builder
	body.WriteString(`<h1>Section Heading</h1>`)
	body.WriteString(`<p>This is a <strong>bold</strong> and <em>italic</em> paragraph with enough text to wrap across multiple lines within the column so word wrapping and pagination are exercised properly.</p>`)
	for range 40 {
		body.WriteString(`<p>Paragraph filler line number to force the document onto several pages so that the manual page-break engine is exercised end to end. Lorem ipsum dolor sit amet, consectetur adipiscing elit.</p>`)
	}
	body.WriteString(`<h2>A Subheading</h2>`)
	body.WriteString(`<ul><li>First bullet item</li><li>Second bullet item that is long enough to wrap onto a second line within the available column width for sure</li></ul>`)
	body.WriteString(`<ol><li>Ordered one</li><li>Ordered two</li></ol>`)
	body.WriteString(`<blockquote><p>A quoted passage rendered in italics with an accent rule beside it.</p></blockquote>`)

	pdf, err := GenerateArticlePDFFromHTML("Smoke Test Headline", "https://example.com/article", body.String())
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(pdf) < 2000 {
		t.Fatalf("pdf unexpectedly small: %d bytes", len(pdf))
	}
	if !strings.HasPrefix(string(pdf[:5]), "%PDF-") {
		t.Fatalf("output is not a PDF: %q", pdf[:8])
	}
	if pages := strings.Count(string(pdf), "/Type /Page"); pages < 2 {
		t.Fatalf("expected multi-page output, got %d page objects", pages)
	}
	if out := os.Getenv("PDF_OUT"); out != "" {
		if werr := os.WriteFile(out, pdf, 0o644); werr != nil {
			t.Fatalf("write out: %v", werr)
		}
	}
}
