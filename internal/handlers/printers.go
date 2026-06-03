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
	"rss-print/internal/services"

	"xorm.io/xorm"
)

type PrinterDiscoverer func(context.Context) ([]models.Printer, error)

// PrinterHandler renders the printer list page.
type PrinterHandler struct {
	DB               *xorm.Engine
	Tmpl             *template.Template
	DiscoverPrinters PrinterDiscoverer
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

	count, err := h.DB.Count(new(models.Printer))
	if err != nil {
		log.Printf("failed to count printers: %v", err)
		h.renderPrintersWithMessage(w, r, "", "Could not save printer", nil)
		return
	}

	var existing models.Printer
	has, err := h.DB.Where("uri = ?", uri).Get(&existing)
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
		if _, err := h.DB.ID(existing.ID).Cols("name", "host", "port", "uri", "updated_at").Update(&existing); err != nil {
			log.Printf("failed to update printer: %v", err)
			h.renderPrintersWithMessage(w, r, "", "Could not update printer", nil)
			return
		}
		h.renderPrintersWithMessage(w, r, "Printer updated", "", nil)
		return
	}

	printer := &models.Printer{Name: name, Host: host, Port: port, URI: uri, IsDefault: count == 0}
	if _, err := h.DB.Insert(printer); err != nil {
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
	var printer models.Printer
	has, err := h.DB.ID(id).Get(&printer)
	if err != nil || !has {
		h.renderPrintersWithMessage(w, r, "", "Printer not found", nil)
		return
	}
	if _, err := h.DB.Exec("UPDATE printer SET is_default = 0"); err != nil {
		log.Printf("failed to clear default printer: %v", err)
		h.renderPrintersWithMessage(w, r, "", "Could not set default printer", nil)
		return
	}
	printer.IsDefault = true
	if _, err := h.DB.ID(printer.ID).Cols("is_default", "updated_at").Update(&printer); err != nil {
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

func (h *PrinterHandler) printerData(r *http.Request, notice string, formError string, discovered []models.Printer) (map[string]any, error) {
	var printers []models.Printer
	if err := h.DB.OrderBy("is_default DESC, created_at DESC").Find(&printers); err != nil {
		return nil, err
	}
	return map[string]any{
		"User":       middleware.GetUser(r.Context()),
		"Path":       "/printers",
		"Printers":   printers,
		"Discovered": discovered,
		"Notice":     notice,
		"Error":      formError,
	}, nil
}
