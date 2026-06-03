package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strconv"

	"rss-print/internal/middleware"
	"rss-print/internal/models"

	"xorm.io/xorm"
)

type PrintJobView struct {
	Job     models.PrintJob
	Article models.Article
	Printer models.Printer
}

// DashboardHandler renders the main dashboard
type DashboardHandler struct {
	DB   *xorm.Engine
	Tmpl *template.Template
}

func (h *DashboardHandler) Render(w http.ResponseWriter, r *http.Request) {
	data, err := h.dashboardData(r, "", "")
	if err != nil {
		log.Printf("failed to load dashboard data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := renderPage(w, r, h.Tmpl, data); err != nil {
		log.Printf("failed to render dashboard template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *DashboardHandler) HandleCreatePrint(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderDashboardWithMessage(w, r, "", "Invalid form submission")
		return
	}

	articleID, err := strconv.ParseInt(r.FormValue("article_id"), 10, 64)
	if err != nil || articleID <= 0 {
		h.renderDashboardWithMessage(w, r, "", "Choose an article to print")
		return
	}

	var article models.Article
	has, err := h.DB.ID(articleID).Get(&article)
	if err != nil || !has {
		h.renderDashboardWithMessage(w, r, "", "Article not found")
		return
	}

	printerID, err := parseOptionalInt64(r.FormValue("printer_id"))
	if err != nil {
		h.renderDashboardWithMessage(w, r, "", "Invalid printer")
		return
	}
	if printerID > 0 {
		var printer models.Printer
		has, err := h.DB.ID(printerID).Get(&printer)
		if err != nil || !has {
			h.renderDashboardWithMessage(w, r, "", "Printer not found")
			return
		}
	}

	job := &models.PrintJob{ArticleID: articleID, PrinterID: printerID, Status: "Pending"}
	if _, err := h.DB.Insert(job); err != nil {
		log.Printf("failed to create print job: %v", err)
		h.renderDashboardWithMessage(w, r, "", "Could not queue print job")
		return
	}

	h.renderDashboardWithMessage(w, r, "Print job queued", "")
}

func (h *DashboardHandler) HandleRetryPrint(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderDashboardWithMessage(w, r, "", "Invalid print job")
		return
	}

	var job models.PrintJob
	has, err := h.DB.ID(id).Get(&job)
	if err != nil || !has {
		h.renderDashboardWithMessage(w, r, "", "Print job not found")
		return
	}
	if job.Status != "Failed" {
		h.renderDashboardWithMessage(w, r, "", "Only failed jobs can be retried")
		return
	}

	job.Status = "Pending"
	job.LastError = ""
	if _, err := h.DB.ID(job.ID).Cols("status", "last_error", "updated_at").Update(&job); err != nil {
		log.Printf("failed to retry print job: %v", err)
		h.renderDashboardWithMessage(w, r, "", "Could not retry print job")
		return
	}

	h.renderDashboardWithMessage(w, r, "Print job reset to pending", "")
}

func (h *DashboardHandler) renderDashboardWithMessage(w http.ResponseWriter, r *http.Request, notice string, formError string) {
	data, err := h.dashboardData(r, notice, formError)
	if err != nil {
		log.Printf("failed to load dashboard data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := renderPage(w, r, h.Tmpl, data); err != nil {
		log.Printf("failed to render dashboard template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *DashboardHandler) dashboardData(r *http.Request, notice string, formError string) (map[string]any, error) {
	var jobs []models.PrintJob
	if err := h.DB.OrderBy("created_at DESC").Limit(10).Find(&jobs); err != nil {
		return nil, err
	}

	jobViews := make([]PrintJobView, 0, len(jobs))
	for _, job := range jobs {
		view := PrintJobView{Job: job}
		_, _ = h.DB.ID(job.ArticleID).Get(&view.Article)
		if job.PrinterID > 0 {
			_, _ = h.DB.ID(job.PrinterID).Get(&view.Printer)
		}
		jobViews = append(jobViews, view)
	}

	var articles []models.Article
	if err := h.DB.OrderBy("created_at DESC").Limit(10).Find(&articles); err != nil {
		return nil, err
	}

	var printers []models.Printer
	if err := h.DB.OrderBy("is_default DESC, name ASC").Find(&printers); err != nil {
		return nil, err
	}

	return map[string]any{
		"User":     middleware.GetUser(r.Context()),
		"Path":     "/",
		"Notice":   notice,
		"Error":    formError,
		"Jobs":     jobViews,
		"Articles": articles,
		"Printers": printers,
	}, nil
}
