package services

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/signintech/gopdf"
	"golang.org/x/net/html"
	"rss-print/ui"
)

// GeneratePDF creates a simple PDF document from article text
func GeneratePDF(title, content string) ([]byte, error) {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.AddPage()

	// Load font from embedded FS
	fontBytes, err := ui.FS.ReadFile("static/fonts/Roboto-Regular.ttf")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded font: %w", err)
	}

	err = pdf.AddTTFFontByReader("Roboto", bytes.NewReader(fontBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to add font: %w", err)
	}

	err = pdf.SetFont("Roboto", "", 16)
	if err != nil {
		return nil, err
	}

	// Print Title
	pdf.Cell(nil, title)
	pdf.Br(20)

	// Switch to body font size
	pdf.SetFont("Roboto", "", 12)

	// Very basic HTML to text stripping
	cleanText := stripHTML(content)

	// Print body (MultiCell handles wrapping)
	// Create a rect for the multicell
	err = pdf.MultiCell(&gopdf.Rect{W: 190, H: 800}, cleanText)
	if err != nil {
		return nil, fmt.Errorf("failed to write content: %w", err)
	}

	var b bytes.Buffer
	_, err = pdf.WriteTo(&b)
	if err != nil {
		return nil, fmt.Errorf("failed to generate pdf buffer: %w", err)
	}

	return b.Bytes(), nil
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
