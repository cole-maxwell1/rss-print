package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strconv"

	"rss-print/internal/middleware"
	"rss-print/internal/models"
	"rss-print/internal/repositories"
	"rss-print/internal/services"
)

type PrintJobView struct {
	Job     models.PrintJob
	Article models.Article
	Printer models.Printer
}

// DashboardHandler renders the main dashboard
type DashboardHandler struct {
	Articles *repositories.ArticleRepo
	Printers *repositories.PrinterRepo
	Jobs     *repositories.PrintJobRepo
	Tmpl     *template.Template
}

type dashboardPageData struct {
	pageData
	Jobs     []PrintJobView
	Articles []models.Article
	Printers []models.Printer
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

	_, has, err := h.Articles.GetByID(articleID)
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
		_, has, err := h.Printers.GetByID(printerID)
		if err != nil || !has {
			h.renderDashboardWithMessage(w, r, "", "Printer not found")
			return
		}
	}

	job := &models.PrintJob{ArticleID: articleID, PrinterID: printerID, Status: "Pending"}
	if err := h.Jobs.Create(job); err != nil {
		log.Printf("failed to create print job: %v", err)
		h.renderDashboardWithMessage(w, r, "", "Could not queue print job")
		return
	}

	h.renderDashboardWithMessage(w, r, "Print job queued", "")
}

func (h *DashboardHandler) HandleDownloadPDF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid article", http.StatusBadRequest)
		return
	}

	article, has, err := h.Articles.GetByID(id)
	if err != nil || !has {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	pdfBytes, err := services.GenerateArticlePDF(article.Title, article.URL)
	if err != nil {
		log.Printf("failed to generate article pdf: %v", err)
		http.Error(w, "Could not generate PDF", http.StatusInternalServerError)
		return
	}

	filename := pdfFilename(article.Title)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfBytes)))
	w.Write(pdfBytes)
}

func (h *DashboardHandler) HandleRetryPrint(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderDashboardWithMessage(w, r, "", "Invalid print job")
		return
	}

	job, has, err := h.Jobs.GetByID(id)
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
	if err := h.Jobs.UpdateStatus(job); err != nil {
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

func (h *DashboardHandler) dashboardData(r *http.Request, notice string, formError string) (dashboardPageData, error) {
	jobs, err := h.Jobs.ListRecent(10)
	if err != nil {
		return dashboardPageData{}, err
	}

	jobViews := make([]PrintJobView, 0, len(jobs))
	for _, job := range jobs {
		view := PrintJobView{Job: job}
		if article, has, _ := h.Articles.GetByID(job.ArticleID); has {
			view.Article = *article
		}
		if job.PrinterID > 0 {
			if printer, has, _ := h.Printers.GetByID(job.PrinterID); has {
				view.Printer = *printer
			}
		}
		jobViews = append(jobViews, view)
	}

	articles, err := h.Articles.ListRecent(10)
	if err != nil {
		return dashboardPageData{}, err
	}

	printers, err := h.Printers.List()
	if err != nil {
		return dashboardPageData{}, err
	}

	return dashboardPageData{
		pageData: pageData{
			User:   middleware.GetUser(r.Context()),
			Path:   "/",
			Notice: notice,
			Error:  formError,
		},
		Jobs:     jobViews,
		Articles: articles,
		Printers: printers,
	}, nil
}
