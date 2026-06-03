package services

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/phin1x/go-ipp"
)

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

// PrintPDF sends a raw PDF byte buffer to an IPP printer URI.
//
// We deliberately use the low-level go-ipp request API rather than the
// client.PrintJob helper: that helper POSTs to /printers/<name> and advertises a
// printer-uri of ipp://localhost/printers/<name>, neither of which matches an
// AirPrint/Mopria device living at /ipp/print. Building the request by hand lets
// us send the real printer-uri to the real endpoint.
//
// The request is attempted as IPP/2.0 first (go-ipp's default). If the printer
// rejects it with 0x0400 client-error-bad-request — the signature of a version
// mismatch — we retry as IPP/1.1.
func PrintPDF(printerURI string, pdfBytes []byte, jobName string) error {
	host, port, useTLS, postURL, err := parsePrinterURI(printerURI)
	if err != nil {
		return err
	}
	log.Printf("PrintPDF: resolved printer URI %q -> POST %q", printerURI, postURL)

	safeJobName := sanitiseJobName(jobName)
	client := ipp.NewIPPClient(host, port, "", "", useTLS)

	// Try IPP/2.0 first, then fall back to 1.1 only on client-error-bad-request.
	versions := [][2]int8{{2, 0}, {1, 1}}
	var lastErr error
	for _, v := range versions {
		req := ipp.NewRequest(ipp.OperationPrintJob, 1)
		req.ProtocolVersionMajor, req.ProtocolVersionMinor = v[0], v[1]
		req.OperationAttributes[ipp.AttributePrinterURI] = printerURI
		req.OperationAttributes[ipp.AttributeRequestingUserName] = "rss-print"
		req.OperationAttributes[ipp.AttributeJobName] = safeJobName
		// application/octet-stream: the printer's document-format-supported does not
		// include application/pdf — octet-stream lets it auto-sense the PDF.
		req.OperationAttributes[ipp.AttributeDocumentFormat] = ipp.MimeTypeOctetStream
		req.JobAttributes[ipp.AttributeSides] = "one-sided"
		// FileSize set so the adapter sends a fixed Content-Length (no chunking,
		// which many older Brother models do not support).
		req.File = bytes.NewReader(pdfBytes)
		req.FileSize = len(pdfBytes)

		if _, err := client.SendRequest(postURL, req, nil); err == nil {
			log.Printf("PrintPDF: job %q accepted by printer (IPP/%d.%d)", safeJobName, v[0], v[1])
			return nil
		} else {
			lastErr = err
			var ippErr ipp.IPPError
			if errors.As(err, &ippErr) {
				log.Printf("IPP/%d.%d error 0x%04x for job %q: %s", v[0], v[1], ippErr.Status, safeJobName, ippErr.Message)
				if ippErr.Status == statusClientErrorBadRequest {
					continue // likely a version mismatch — try the next version
				}
				return fmt.Errorf("ipp print failed (IPP/%d.%d, status 0x%04x): %w", v[0], v[1], ippErr.Status, err)
			}
			// Transport / HTTP error — not a version problem; let the worker's
			// retry/backoff handle it rather than re-trying another version here.
			return fmt.Errorf("ipp post failed (IPP/%d.%d): %w", v[0], v[1], err)
		}
	}

	return fmt.Errorf("ipp print job %q rejected as bad-request under both IPP/2.0 and IPP/1.1: %w", safeJobName, lastErr)
}
