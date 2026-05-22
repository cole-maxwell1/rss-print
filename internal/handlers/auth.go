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

	"github.com/mmcdole/gofeed"
	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"
)

type FeedFetcher func(context.Context, string, string, string) (*gofeed.Feed, error)
type PrinterDiscoverer func(context.Context) ([]models.Printer, error)

type PrintJobView struct {
	Job     models.PrintJob
	Article models.Article
	Printer models.Printer
}

type AuthHandler struct {
	DB   *xorm.Engine
	Tmpl *template.Template
}

func (h *AuthHandler) RenderLogin(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}
	if r.URL.Query().Get("error") == "1" {
		data["Error"] = "Invalid username or password"
	}
	if err := h.Tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("failed to render login template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user := &models.User{}
	has, err := h.DB.Where("username = ?", username).Get(user)
	if err != nil || !has {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}

	session, _ := middleware.Store.Get(r, "session-name")
	session.Values["userID"] = user.ID
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := middleware.Store.Get(r, "session-name")
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// CreateDefaultUser creates a default admin user if no users exist
func CreateDefaultUser(db *xorm.Engine) error {
	count, err := db.Count(new(models.User))
	if err != nil {
		return err
	}
	if count == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		user := &models.User{
			Username:     "admin",
			PasswordHash: string(hash),
		}
		_, err = db.Insert(user)
		if err != nil {
			return err
		}
		log.Println("Created default user: admin / admin")
	}
	return nil
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

// FeedHandler renders the feed list page.
type FeedHandler struct {
	DB        *xorm.Engine
	Tmpl      *template.Template
	FetchFeed FeedFetcher
}

func (h *FeedHandler) Render(w http.ResponseWriter, r *http.Request) {
	data, err := h.feedData(r, "", "")
	if err != nil {
		log.Printf("failed to load feeds data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := renderPage(w, r, h.Tmpl, data); err != nil {
		log.Printf("failed to render feeds template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *FeedHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderFeedsWithMessage(w, r, "", "Invalid form submission")
		return
	}

	feedURL := strings.TrimSpace(r.FormValue("url"))
	parsedURL, err := url.ParseRequestURI(feedURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		h.renderFeedsWithMessage(w, r, "", "Enter a valid HTTP or HTTPS feed URL")
		return
	}

	authHeaderName := strings.TrimSpace(r.FormValue("auth_header_name"))
	authHeaderValue := strings.TrimSpace(r.FormValue("auth_header_value"))
	if (authHeaderName == "") != (authHeaderValue == "") {
		h.renderFeedsWithMessage(w, r, "", "Custom auth header name and value must both be provided")
		return
	}
	if strings.ContainsAny(authHeaderName, " \t\r\n:") {
		h.renderFeedsWithMessage(w, r, "", "Custom auth header name is invalid")
		return
	}

	pollInterval, err := strconv.Atoi(strings.TrimSpace(r.FormValue("poll_interval")))
	if err != nil || pollInterval < 1 {
		h.renderFeedsWithMessage(w, r, "", "Poll interval must be at least 1 minute")
		return
	}

	printerID, err := parseOptionalInt64(r.FormValue("printer_id"))
	if err != nil {
		h.renderFeedsWithMessage(w, r, "", "Invalid feed printer")
		return
	}
	if printerID > 0 {
		var printer models.Printer
		has, err := h.DB.ID(printerID).Get(&printer)
		if err != nil || !has {
			h.renderFeedsWithMessage(w, r, "", "Feed printer not found")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	fetcher := h.FetchFeed
	if fetcher == nil {
		fetcher = services.FetchFeed
	}
	parsedFeed, err := fetcher(ctx, feedURL, authHeaderName, authHeaderValue)
	if err != nil {
		h.renderFeedsWithMessage(w, r, "", "Could not fetch feed: "+err.Error())
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSpace(parsedFeed.Title)
	}
	if name == "" {
		name = parsedURL.Host
	}

	feed := &models.Feed{
		Name:            name,
		URL:             feedURL,
		AuthHeaderName:  authHeaderName,
		AuthHeaderValue: authHeaderValue,
		AutoPrint:       r.FormValue("auto_print") == "on",
		PrinterID:       printerID,
		PollInterval:    pollInterval,
		LastPolledAt:    time.Now(),
	}
	if _, err := h.DB.Insert(feed); err != nil {
		log.Printf("failed to save feed: %v", err)
		h.renderFeedsWithMessage(w, r, "", "Could not save feed; it may already exist")
		return
	}

	imported := importFeedArticles(h.DB, feed.ID, parsedFeed)
	h.renderFeedsWithMessage(w, r, "Feed added with "+strconv.Itoa(imported)+" current articles imported", "")
}

func (h *FeedHandler) renderFeedsWithMessage(w http.ResponseWriter, r *http.Request, notice string, formError string) {
	data, err := h.feedData(r, notice, formError)
	if err != nil {
		log.Printf("failed to load feeds data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := renderPage(w, r, h.Tmpl, data); err != nil {
		log.Printf("failed to render feeds template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *FeedHandler) feedData(r *http.Request, notice string, formError string) (map[string]any, error) {
	var feeds []models.Feed
	if err := h.DB.OrderBy("created_at DESC").Find(&feeds); err != nil {
		return nil, err
	}
	var printers []models.Printer
	if err := h.DB.OrderBy("is_default DESC, name ASC").Find(&printers); err != nil {
		return nil, err
	}
	return map[string]any{
		"User":     middleware.GetUser(r.Context()),
		"Path":     r.URL.Path,
		"Feeds":    feeds,
		"Printers": printers,
		"Notice":   notice,
		"Error":    formError,
	}, nil
}

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

func parseOptionalInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func importFeedArticles(engine *xorm.Engine, feedID int64, parsedFeed *gofeed.Feed) int {
	imported := 0
	for _, item := range parsedFeed.Items {
		guid := strings.TrimSpace(item.GUID)
		if guid == "" {
			guid = strings.TrimSpace(item.Link)
		}
		if guid == "" {
			guid = strings.TrimSpace(item.Title)
		}
		if guid == "" {
			continue
		}

		article := &models.Article{
			FeedID: feedID,
			GUID:   guid,
			Title:  strings.TrimSpace(item.Title),
			URL:    strings.TrimSpace(item.Link),
		}
		if article.Title == "" {
			article.Title = article.URL
		}
		if item.PublishedParsed != nil {
			article.PublishedAt = *item.PublishedParsed
		}
		if _, err := engine.Insert(article); err == nil {
			imported++
		}
	}
	return imported
}

func renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, data map[string]any) error {
	if r.Header.Get("HX-Request") == "true" {
		if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
			return err
		}
		return tmpl.ExecuteTemplate(w, "nav", data)
	}
	return tmpl.ExecuteTemplate(w, "base.html", data)
}
