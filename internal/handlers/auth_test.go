package handlers

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"rss-print/internal/db"
	"rss-print/internal/middleware"
	"rss-print/internal/models"
	"rss-print/ui"

	"github.com/mmcdole/gofeed"
	"xorm.io/xorm"
)

func TestFeedCreateValidatesCustomHeaderPair(t *testing.T) {
	engine := testDB(t)
	tmpl := testTemplate(t, "templates/feeds.html")
	called := false
	handler := &FeedHandler{
		DB:   engine,
		Tmpl: tmpl,
		FetchFeed: func(context.Context, string, string, string) (*gofeed.Feed, error) {
			called = true
			return &gofeed.Feed{}, nil
		},
	}

	req := testFormRequest(t, "/feeds", url.Values{
		"url":              {"https://example.com/feed.xml"},
		"poll_interval":    {"30"},
		"auth_header_name": {"X-Token"},
	})
	rr := httptest.NewRecorder()

	handler.HandleCreate(rr, req)

	if called {
		t.Fatal("fetcher should not be called when custom header value is missing")
	}
	if !strings.Contains(rr.Body.String(), "Custom auth header name and value must both be provided") {
		t.Fatalf("expected validation error in response, got %q", rr.Body.String())
	}
}

func TestFeedCreateImportsBaselineArticlesWithoutJobs(t *testing.T) {
	engine := testDB(t)
	tmpl := testTemplate(t, "templates/feeds.html")
	handler := &FeedHandler{
		DB:   engine,
		Tmpl: tmpl,
		FetchFeed: func(_ context.Context, feedURL string, headerName string, headerValue string) (*gofeed.Feed, error) {
			if feedURL != "https://example.com/feed.xml" || headerName != "X-Token" || headerValue != "secret" {
				t.Fatalf("unexpected fetch args: %q %q %q", feedURL, headerName, headerValue)
			}
			published := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
			return &gofeed.Feed{
				Title: "Example Feed",
				Items: []*gofeed.Item{
					{Title: "One", Link: "https://example.com/one", GUID: "one", PublishedParsed: &published},
					{Title: "Two", Link: "https://example.com/two", GUID: "two"},
				},
			}, nil
		},
	}

	req := testFormRequest(t, "/feeds", url.Values{
		"url":               {"https://example.com/feed.xml"},
		"poll_interval":     {"15"},
		"auto_print":        {"on"},
		"auth_header_name":  {"X-Token"},
		"auth_header_value": {"secret"},
	})
	rr := httptest.NewRecorder()

	handler.HandleCreate(rr, req)

	var feed models.Feed
	has, err := engine.Where("url = ?", "https://example.com/feed.xml").Get(&feed)
	if err != nil || !has {
		t.Fatalf("expected feed to be saved: has=%v err=%v", has, err)
	}
	if feed.AuthHeaderName != "X-Token" || feed.AuthHeaderValue != "secret" || !feed.AutoPrint || feed.PollInterval != 15 {
		t.Fatalf("saved feed fields did not match: %+v", feed)
	}

	articles, err := engine.Count(new(models.Article))
	if err != nil {
		t.Fatal(err)
	}
	if articles != 2 {
		t.Fatalf("expected 2 imported articles, got %d", articles)
	}

	jobs, err := engine.Count(new(models.PrintJob))
	if err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("expected no print jobs for baseline imports, got %d", jobs)
	}
}

func TestPrinterCreateUpsertsByURI(t *testing.T) {
	engine := testDB(t)
	handler := &PrinterHandler{DB: engine, Tmpl: testTemplate(t, "templates/printers.html")}

	handler.HandleCreate(httptest.NewRecorder(), testFormRequest(t, "/printers", url.Values{
		"name": {"Office"}, "host": {"printer.local"}, "port": {"631"}, "uri": {"ipp://printer.local/ipp/print"},
	}))
	handler.HandleCreate(httptest.NewRecorder(), testFormRequest(t, "/printers", url.Values{
		"name": {"Office Updated"}, "host": {"printer.local"}, "port": {"631"}, "uri": {"ipp://printer.local/ipp/print"},
	}))

	count, err := engine.Count(new(models.Printer))
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one printer after upsert, got %d", count)
	}

	var printer models.Printer
	has, err := engine.Where("uri = ?", "ipp://printer.local/ipp/print").Get(&printer)
	if err != nil || !has {
		t.Fatalf("expected printer: has=%v err=%v", has, err)
	}
	if printer.Name != "Office Updated" || !printer.IsDefault {
		t.Fatalf("unexpected printer after upsert: %+v", printer)
	}
}

func TestPrinterSetDefaultClearsOtherDefaults(t *testing.T) {
	engine := testDB(t)
	first := &models.Printer{Name: "First", Host: "one.local", Port: 631, URI: "ipp://one.local/ipp/print", IsDefault: true}
	second := &models.Printer{Name: "Second", Host: "two.local", Port: 631, URI: "ipp://two.local/ipp/print"}
	if _, err := engine.Insert(first, second); err != nil {
		t.Fatal(err)
	}
	handler := &PrinterHandler{DB: engine, Tmpl: testTemplate(t, "templates/printers.html")}

	req := testFormRequest(t, "/printers/2/default", nil)
	req.SetPathValue("id", "2")
	handler.HandleSetDefault(httptest.NewRecorder(), req)

	var printers []models.Printer
	if err := engine.OrderBy("id ASC").Find(&printers); err != nil {
		t.Fatal(err)
	}
	if printers[0].IsDefault || !printers[1].IsDefault {
		t.Fatalf("expected only second printer to be default: %+v", printers)
	}
}

func TestManualPrintCreatesPendingJob(t *testing.T) {
	engine := testDB(t)
	article := &models.Article{FeedID: 1, GUID: "one", Title: "One", URL: "https://example.com/one"}
	printer := &models.Printer{Name: "Office", Host: "printer.local", Port: 631, URI: "ipp://printer.local/ipp/print", IsDefault: true}
	if _, err := engine.Insert(article, printer); err != nil {
		t.Fatal(err)
	}
	handler := &DashboardHandler{DB: engine, Tmpl: testTemplate(t, "templates/dashboard.html")}

	req := testFormRequest(t, "/prints", url.Values{
		"article_id": {strconvFormat(article.ID)},
		"printer_id": {strconvFormat(printer.ID)},
	})
	handler.HandleCreatePrint(httptest.NewRecorder(), req)

	var job models.PrintJob
	has, err := engine.Where("article_id = ?", article.ID).Get(&job)
	if err != nil || !has {
		t.Fatalf("expected print job: has=%v err=%v", has, err)
	}
	if job.Status != "Pending" || job.PrinterID != printer.ID {
		t.Fatalf("unexpected job: %+v", job)
	}
}

func testDB(t *testing.T) *xorm.Engine {
	t.Helper()
	engine, err := db.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	return engine
}

func testTemplate(t *testing.T, page string) *template.Template {
	t.Helper()
	return template.Must(template.ParseFS(ui.FS, "templates/base.html", page))
}

func testFormRequest(t *testing.T, target string, values url.Values) *http.Request {
	t.Helper()
	body := ""
	if values != nil {
		body = values.Encode()
	}
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	user := &models.User{ID: 1, Username: "admin"}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, user))
}

func strconvFormat(value int64) string {
	return strconv.FormatInt(value, 10)
}
