package services

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/OpenPrinting/goipp"
)

// PrintPDF sends a raw PDF byte buffer to an IPP printer URI
func PrintPDF(printerURI string, pdfBytes []byte, jobName string) error {
	m := goipp.NewRequest(goipp.DefaultVersion, goipp.OpPrintJob, 1)
	m.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
	m.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-US")))
	m.Operation.Add(goipp.MakeAttribute("printer-uri", goipp.TagURI, goipp.String(printerURI)))
	m.Operation.Add(goipp.MakeAttribute("requesting-user-name", goipp.TagName, goipp.String("rss-print")))
	m.Operation.Add(goipp.MakeAttribute("job-name", goipp.TagName, goipp.String(jobName)))
	m.Operation.Add(goipp.MakeAttribute("document-format", goipp.TagMimeType, goipp.String("application/pdf")))

	// Set IPP attributes (basic duplex)
	m.Job.Add(goipp.MakeAttribute("sides", goipp.TagKeyword, goipp.String("two-sided-long-edge")))

	reqBytes, err := m.EncodeBytes()
	if err != nil {
		return fmt.Errorf("failed to encode ipp request: %w", err)
	}

	body := io.MultiReader(bytes.NewReader(reqBytes), bytes.NewReader(pdfBytes))

	req, err := http.NewRequest("POST", printerURI, body)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/ipp")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ipp post failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("printer returned non-200 status: %d", resp.StatusCode)
	}

	var respMsg goipp.Message
	err = respMsg.Decode(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to decode ipp response: %w", err)
	}

	// Check IPP status code
	if goipp.Status(respMsg.Code) != goipp.StatusOk {
		return fmt.Errorf("ipp print job failed with status: 0x%04x", respMsg.Code)
	}

	return nil
}
