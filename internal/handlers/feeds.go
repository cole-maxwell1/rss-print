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
	"xorm.io/xorm"
)

type FeedFetcher func(context.Context, string, string, string) (*gofeed.Feed, error)

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
