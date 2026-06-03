package services

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/grandcat/zeroconf"
	"rss-print/internal/models"
)

// ippPathFromTXT extracts the IPP resource path from the mDNS TXT records published
// by a printer. The "rp" key holds the correct path (e.g. "ipp/print" or "ipp").
// If the key is absent we fall back to "/ipp/print" as a best-guess default.
func ippPathFromTXT(txtRecords []string) string {
	for _, record := range txtRecords {
		if strings.HasPrefix(record, "rp=") {
			path := strings.TrimPrefix(record, "rp=")
			// Normalise: ensure exactly one leading slash.
			return "/" + strings.TrimPrefix(path, "/")
		}
	}
	// Fallback — common default for most IPP printers.
	log.Println("DiscoverPrinters: no 'rp' TXT record found, falling back to /ipp/print")
	return "/ipp/print"
}

// DiscoverPrinters scans the local network via mDNS for IPP printers.
// The IPP resource path is read from the printer's "rp" TXT record so that
// the stored URI is correct even when a printer uses a non-standard path.
func DiscoverPrinters(ctx context.Context) ([]models.Printer, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	var printers []models.Printer

	go func(results <-chan *zeroconf.ServiceEntry) {
		for entry := range results {
			if len(entry.AddrIPv4) == 0 {
				continue
			}

			ip := entry.AddrIPv4[0].String()
			path := ippPathFromTXT(entry.Text)
			uri := fmt.Sprintf("ipp://%s:%d%s", ip, entry.Port, path)

			log.Printf("DiscoverPrinters: found %q at %s:%d — URI: %s (rp path: %s, TXT: %v)",
				entry.Instance, ip, entry.Port, uri, path, entry.Text)

			printers = append(printers, models.Printer{
				Name: entry.Instance,
				Host: ip,
				Port: entry.Port,
				URI:  uri,
			})
		}
	}(entries)

	// Browse for all IPP services on the local network.
	if err = resolver.Browse(ctx, "_ipp._tcp", "local.", entries); err != nil {
		return nil, fmt.Errorf("failed to browse: %w", err)
	}

	<-ctx.Done() // Wait for the caller-supplied timeout.

	return printers, nil
}