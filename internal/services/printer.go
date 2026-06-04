package services

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/phin1x/go-ipp"
)

// rasterDPI is the resolution at which articles are rasterized for raster-only
// printers. 600 matches the Brother MFC-L3770CDW's only advertised
// pwg-raster-document-resolution-supported value; printers that advertise other
// resolutions still accept this and scale to their native grid.
const rasterDPI = 600

// statusClientErrorBadRequest is IPP status 0x0400 (client-error-bad-request).
// Brother printers return this when they cannot parse a request — most often a
// protocol-version mismatch. We use it to decide whether to retry with a
// different IPP version.
const statusClientErrorBadRequest int16 = 0x0400

// sanitiseJobName replaces characters that some Brother firmware versions reject in
// job-name values. Only alphanumerics, hyphens, underscores, and spaces are kept.
func sanitiseJobName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return strings.TrimSpace(b.String())
}

// parsePrinterURI derives the host, port, TLS flag, and HTTP POST endpoint from an
// ipp:// or ipps:// printer URI. The returned postURL is what we POST the IPP
// request to (http/https), while the original ipp:// URI is still sent as the
// printer-uri operation attribute.
func parsePrinterURI(printerURI string) (host string, port int, useTLS bool, postURL string, err error) {
	u, err := url.Parse(printerURI)
	if err != nil {
		return "", 0, false, "", fmt.Errorf("invalid printer URI %q: %w", printerURI, err)
	}

	var httpScheme string
	switch u.Scheme {
	case "ipp":
		useTLS, httpScheme = false, "http"
	case "ipps":
		useTLS, httpScheme = true, "https"
	default:
		return "", 0, false, "", fmt.Errorf("unsupported printer URI scheme: %q (expected ipp:// or ipps://)", printerURI)
	}

	host = u.Hostname()
	if host == "" {
		return "", 0, false, "", fmt.Errorf("printer URI %q has no host", printerURI)
	}

	port = 631
	if p := u.Port(); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			return "", 0, false, "", fmt.Errorf("invalid port in printer URI %q: %w", printerURI, err)
		}
	}

	postURL = fmt.Sprintf("%s://%s:%d%s", httpScheme, host, port, u.Path)
	return host, port, useTLS, postURL, nil
}

// printCandidate is one (document-format, document-body) pairing PrintDocument may
// attempt. body is a thunk so an expensive transform (rasterisation) runs only
// if that candidate is actually reached.
type printCandidate struct {
	format string
	body   func() ([]byte, error)
}

// PrintDocument sends an article to an IPP printer in a format the printer can
// render, chosen from its document-format-supported list:
//
//   - application/pdf — the prebuilt PDF, for printers with a PDF interpreter.
//   - image/pwg-raster — the article rasterized to PWG Raster in pure Go, the
//     path raster-only devices such as the Brother MFC-L3770CDW require, since
//     they expose no PDF interpreter and would otherwise print the raw PDF bytes
//     as text.
//   - image/jpeg — a single-page JPEG, last-resort fallback.
//
// Raster output is rendered directly from the article HTML (doc.CleanHTML), not
// by rasterizing the PDF, since there is no pure-Go PDF rasterizer. Rendering is
// memoized so a PWG attempt followed by a JPEG fallback renders the bitmap once.
//
// The low-level go-ipp request API is used rather than the client.PrintJob
// helper: that helper POSTs to /printers/<name> and advertises a printer-uri of
// ipp://localhost/printers/<name>, neither of which matches an AirPrint/Mopria
// device living at /ipp/print. Building the request by hand sends the real
// printer-uri to the real endpoint.
func PrintDocument(printerURI string, doc *ArticleDocument, jobName string) error {
	host, port, useTLS, postURL, err := parsePrinterURI(printerURI)
	if err != nil {
		return err
	}
	log.Printf("PrintDocument: resolved printer URI %q -> POST %q", printerURI, postURL)

	safeJobName := sanitiseJobName(jobName)
	client := ipp.NewIPPClient(host, port, "", "", useTLS)

	// renderImages rasterizes the article once and caches the result, so the PWG
	// and JPEG candidates share a single (expensive) render.
	var (
		cachedImgs []image.Image
		cachedErr  error
		didRender  bool
	)
	renderImages := func() ([]image.Image, error) {
		if !didRender {
			cachedImgs, cachedErr = renderArticleToImages(doc.Title, doc.BaseURL, doc.CleanHTML, rasterDPI)
			didRender = true
		}
		return cachedImgs, cachedErr
	}

	rawPDF := func() ([]byte, error) { return doc.PDF, nil }
	toPWG := func() ([]byte, error) {
		imgs, err := renderImages()
		if err != nil {
			return nil, err
		}
		return encodePWG(imgs, rasterDPI)
	}
	toJPEG := func() ([]byte, error) {
		imgs, err := renderImages()
		if err != nil {
			return nil, err
		}
		return encodeJPEG(imgs)
	}

	supported, err := fetchPrinterFormats(client, postURL, printerURI)
	var plan []printCandidate
	switch {
	case err != nil:
		// Capabilities unknown: try PDF, then octet-stream as a last resort —
		// the pre-capability behaviour.
		log.Printf("PrintDocument: could not read printer formats (%v); assuming PDF-capable", err)
		plan = []printCandidate{
			{"application/pdf", rawPDF},
			{ipp.MimeTypeOctetStream, rawPDF},
		}
	default:
		log.Printf("PrintDocument: printer supports formats %v", supported)
		if slices.Contains(supported, "application/pdf") {
			plan = append(plan, printCandidate{"application/pdf", rawPDF})
		}
		if slices.Contains(supported, "image/pwg-raster") {
			plan = append(plan, printCandidate{"image/pwg-raster", toPWG})
		}
		if slices.Contains(supported, "image/jpeg") {
			plan = append(plan, printCandidate{"image/jpeg", toJPEG})
		}
		if len(plan) == 0 {
			return fmt.Errorf("printer supports none of the formats rss-print can produce (application/pdf, image/pwg-raster, image/jpeg); advertised: %v", supported)
		}
	}

	var lastErr error
	for _, c := range plan {
		body, err := c.body()
		if err != nil {
			lastErr = fmt.Errorf("preparing %s document: %w", c.format, err)
			log.Printf("PrintDocument: %v", lastErr)
			continue
		}

		err = sendDocument(client, postURL, printerURI, c.format, safeJobName, body)
		if err == nil {
			return nil
		}
		lastErr = err

		// A printer rejecting this format (0x040a) is the only error worth trying
		// the next candidate for; any other error (transport, other IPP status) is
		// not format-related, so stop and let the worker's retry/backoff handle it.
		if ippErr, ok := errors.AsType[ipp.IPPError](err); ok && ippErr.Status == ipp.StatusErrorDocumentFormatNotSupported {
			log.Printf("PrintDocument: printer rejected format %s; trying next candidate", c.format)
			continue
		}
		return err
	}

	return fmt.Errorf("print job %q failed under all candidate formats: %w", safeJobName, lastErr)
}

// fetchPrinterFormats queries the printer for its document-format-supported list.
// The request is built and POSTed by hand for the same reason as PrintDocument: the
// go-ipp client helpers target /printers/<name>, not the device's real endpoint.
func fetchPrinterFormats(client *ipp.IPPClient, postURL, printerURI string) ([]string, error) {
	req := ipp.NewRequest(ipp.OperationGetPrinterAttributes, 1)
	req.OperationAttributes[ipp.AttributePrinterURI] = printerURI
	req.OperationAttributes[ipp.AttributeRequestingUserName] = "rss-print"
	req.OperationAttributes[ipp.AttributeRequestedAttributes] = []string{"document-format-supported"}

	resp, err := client.SendRequest(postURL, req, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.PrinterAttributes) == 0 {
		return nil, errors.New("printer returned no attributes")
	}

	var formats []string
	for _, a := range resp.PrinterAttributes[0]["document-format-supported"] {
		if s, ok := a.Value.(string); ok {
			formats = append(formats, s)
		}
	}
	if len(formats) == 0 {
		return nil, errors.New("printer did not report document-format-supported")
	}
	return formats, nil
}

// sendDocument POSTs one document to the printer as the given format, negotiating
// the IPP protocol version. IPP/2.0 is attempted first (go-ipp's default); a
// 0x0400 client-error-bad-request — the signature of a version mismatch — falls
// back to IPP/1.1. The returned error is the raw IPP error when the printer
// rejects the format (0x040a), so PrintDocument can detect it and try another format.
func sendDocument(client *ipp.IPPClient, postURL, printerURI, format, jobName string, body []byte) error {
	versions := [][2]int8{{2, 0}, {1, 1}}
	var lastErr error
	for _, v := range versions {
		req := ipp.NewRequest(ipp.OperationPrintJob, 1)
		req.ProtocolVersionMajor, req.ProtocolVersionMinor = v[0], v[1]
		req.OperationAttributes[ipp.AttributePrinterURI] = printerURI
		req.OperationAttributes[ipp.AttributeRequestingUserName] = "rss-print"
		req.OperationAttributes[ipp.AttributeJobName] = jobName
		req.OperationAttributes[ipp.AttributeDocumentFormat] = format
		req.JobAttributes[ipp.AttributeSides] = "one-sided"
		// FileSize set so the adapter sends a fixed Content-Length (no chunking,
		// which many older Brother models do not support).
		req.File = bytes.NewReader(body)
		req.FileSize = len(body)

		if _, err := client.SendRequest(postURL, req, nil); err == nil {
			log.Printf("PrintDocument: job %q accepted by printer (format %s, IPP/%d.%d, %d bytes)", jobName, format, v[0], v[1], len(body))
			return nil
		} else {
			lastErr = err
			if ippErr, ok := errors.AsType[ipp.IPPError](err); ok {
				log.Printf("IPP/%d.%d error 0x%04x for job %q (format %s): %s", v[0], v[1], ippErr.Status, jobName, format, ippErr.Message)
				if ippErr.Status == statusClientErrorBadRequest {
					continue // likely a version mismatch — try the next version
				}
				// Return the raw IPP error (unwrapped) so PrintDocument can inspect the
				// status — 0x040a means try the next format.
				return err
			}
			// Transport / HTTP error — not a version or format problem.
			return fmt.Errorf("ipp post failed (format %s, IPP/%d.%d): %w", format, v[0], v[1], err)
		}
	}

	return fmt.Errorf("job %q rejected as bad-request under IPP/2.0 and IPP/1.1: %w", jobName, lastErr)
}
