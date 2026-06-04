// Command printerinfo queries an IPP printer for its attributes and prints them.
//
// It exists to diagnose document-format negotiation: the values of
// document-format-supported, document-format-default, and urf-supported reveal
// what a printer can actually render, which determines how PrintPDF must format
// jobs for it.
//
// Usage:
//
//	go run ./cmd/printerinfo ipp://192.168.1.66:631/ipp/print
//
// The request is built by hand and POSTed to the real printer endpoint, matching
// services.PrintPDF — the go-ipp client helpers target /printers/<name>, which an
// AirPrint/Mopria device at /ipp/print does not expose.
package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strconv"

	"github.com/phin1x/go-ipp"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s ipp://host:port/path", os.Args[0])
	}
	printerURI := os.Args[1]

	u, err := url.Parse(printerURI)
	if err != nil {
		log.Fatalf("invalid printer URI %q: %v", printerURI, err)
	}

	scheme, useTLS := "http", false
	switch u.Scheme {
	case "ipp":
	case "ipps":
		scheme, useTLS = "https", true
	default:
		log.Fatalf("unsupported scheme %q (expected ipp:// or ipps://)", u.Scheme)
	}

	host := u.Hostname()
	port := 631
	if p := u.Port(); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			log.Fatalf("invalid port in %q: %v", printerURI, err)
		}
	}
	postURL := fmt.Sprintf("%s://%s:%d%s", scheme, host, port, u.Path)

	client := ipp.NewIPPClient(host, port, "", "", useTLS)
	req := ipp.NewRequest(ipp.OperationGetPrinterAttributes, 1)
	req.OperationAttributes[ipp.AttributePrinterURI] = printerURI
	req.OperationAttributes[ipp.AttributeRequestingUserName] = "rss-print"
	req.OperationAttributes[ipp.AttributeRequestedAttributes] = []string{"all"}

	resp, err := client.SendRequest(postURL, req, nil)
	if err != nil {
		log.Fatalf("get-printer-attributes failed against %s: %v", postURL, err)
	}
	if len(resp.PrinterAttributes) == 0 {
		log.Fatal("printer returned no attributes")
	}

	attrs := resp.PrinterAttributes[0]
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		values := attrs[name]
		rendered := make([]string, 0, len(values))
		for _, a := range values {
			rendered = append(rendered, fmt.Sprintf("%v", a.Value))
		}
		fmt.Printf("%s: %v\n", name, rendered)
	}
}
