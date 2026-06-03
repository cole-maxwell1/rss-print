package handlers

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rss-print/internal/middleware"
	"rss-print/internal/models"
	"rss-print/internal/repositories"
	"rss-print/internal/services"
)

type PrinterDiscoverer func(context.Context) ([]models.Printer, error)

// PrinterHandler renders the printer list page.
type PrinterHandler struct {
	Printers         *repositories.PrinterRepo
	Tmpl             *template.Template
	DiscoverPrinters PrinterDiscoverer
}

type printersPageData struct {
	pageData
	Printers   []models.Printer
	Discovered []models.Printer
}

func (h *PrinterHandler) Render(w http.ResponseWriter, r *http.Request) {
	data, err := h.printerData(r, "", "", nil)
	if err != nil {
		log.Printf("failed to load printers data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := renderPage(w, r, h.Tmpl, data); err != nil {
		log.Printf("failed to render printers template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *PrinterHandler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	discoverer := h.DiscoverPrinters
	if discoverer == nil {
		discoverer = services.DiscoverPrinters
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	discovered, err := discoverer(ctx)
	if err != nil {
		h.renderPrintersWithMessage(w, r, "", "Discovery failed: "+err.Error(), nil)
		return
	}
	h.renderPrintersWithMessage(w, r, "Discovery found "+strconv.Itoa(len(discovered))+" candidate printers", "", discovered)
}

func (h *PrinterHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrintersWithMessage(w, r, "", "Invalid form submission", nil)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	host := strings.TrimSpace(r.FormValue("host"))
	uri := strings.TrimSpace(r.FormValue("uri"))
	port, err := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
	if name == "" || host == "" || uri == "" || err != nil || port <= 0 {
		h.renderPrintersWithMessage(w, r, "", "Printer name, host, port, and URI are required", nil)
		return
	}
	if parsedURI, err := url.Parse(uri); err != nil || parsedURI.Scheme == "" || parsedURI.Host == "" {
		h.renderPrintersWithMessage(w, r, "", "Printer URI is invalid", nil)
		return
	}

	count, err := h.Printers.Count()
	if err != nil {
		log.Printf("failed to count printers: %v", err)
		h.renderPrintersWithMessage(w, r, "", "Could not save printer", nil)
		return
	}

	existing, has, err := h.Printers.GetByURI(uri)
	if err != nil {
		log.Printf("failed to find printer: %v", err)
		h.renderPrintersWithMessage(w, r, "", "Could not save printer", nil)
		return
	}
	if has {
		existing.Name = name
		existing.Host = host
		existing.Port = port
		existing.URI = uri
		if err := h.Printers.UpdateDetails(existing); err != nil {
			log.Printf("failed to update printer: %v", err)
			h.renderPrintersWithMessage(w, r, "", "Could not update printer", nil)
			return
		}
		h.renderPrintersWithMessage(w, r, "Printer updated", "", nil)
		return
	}

	printer := &models.Printer{Name: name, Host: host, Port: port, URI: uri, IsDefault: count == 0}
	if err := h.Printers.Create(printer); err != nil {
		log.Printf("failed to save printer: %v", err)
		h.renderPrintersWithMessage(w, r, "", "Could not save printer", nil)
		return
	}
	h.renderPrintersWithMessage(w, r, "Printer saved", "", nil)
}

func (h *PrinterHandler) HandleSetDefault(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderPrintersWithMessage(w, r, "", "Invalid printer", nil)
		return
	}
	printer, has, err := h.Printers.GetByID(id)
	if err != nil || !has {
		h.renderPrintersWithMessage(w, r, "", "Printer not found", nil)
		return
	}
	if err := h.Printers.MakeDefault(printer); err != nil {
		log.Printf("failed to set default printer: %v", err)
		h.renderPrintersWithMessage(w, r, "", "Could not set default printer", nil)
		return
	}
	h.renderPrintersWithMessage(w, r, "Default printer updated", "", nil)
}

func (h *PrinterHandler) renderPrintersWithMessage(w http.ResponseWriter, r *http.Request, notice string, formError string, discovered []models.Printer) {
	data, err := h.printerData(r, notice, formError, discovered)
	if err != nil {
		log.Printf("failed to load printers data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := renderPage(w, r, h.Tmpl, data); err != nil {
		log.Printf("failed to render printers template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *PrinterHandler) printerData(r *http.Request, notice string, formError string, discovered []models.Printer) (printersPageData, error) {
	printers, err := h.Printers.ListByCreated()
	if err != nil {
		return printersPageData{}, err
	}
	return printersPageData{
		pageData: pageData{
			User:   middleware.GetUser(r.Context()),
			Path:   "/printers",
			Notice: notice,
			Error:  formError,
		},
		Printers:   printers,
		Discovered: discovered,
	}, nil
}
