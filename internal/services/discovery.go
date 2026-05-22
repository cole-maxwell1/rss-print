package services

import (
	"context"
	"fmt"
	"log"

	"github.com/grandcat/zeroconf"
	"rss-print/internal/models"
)

// DiscoverPrinters scans the local network via mDNS for IPP printers
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
			log.Printf("Discovered Printer: %s (%s:%d)", entry.Instance, entry.AddrIPv4[0], entry.Port)
			// Construct basic URI, note: actual path might be /ipp/print or similar from TXT records
			uri := fmt.Sprintf("ipp://%s:%d/ipp/print", entry.AddrIPv4[0].String(), entry.Port)

			printers = append(printers, models.Printer{
				Name: entry.Instance,
				Host: entry.AddrIPv4[0].String(),
				Port: entry.Port,
				URI:  uri,
			})
		}
	}(entries)

	// Discover all ipp services
	err = resolver.Browse(ctx, "_ipp._tcp", "local.", entries)
	if err != nil {
		return nil, fmt.Errorf("failed to browse: %w", err)
	}

	<-ctx.Done() // Wait for context timeout

	return printers, nil
}
