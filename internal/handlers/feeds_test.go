package handlers

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"rss-print/internal/models"
	"rss-print/internal/repositories"

	"github.com/mmcdole/gofeed"
)

func TestFeedCreateValidatesCustomHeaderPair(t *testing.T) {
	engine := testDB(t)
	tmpl := testTemplate(t, "templates/feeds.html")
	called := false
	handler := &FeedHandler{
		Feeds:    repositories.NewFeedRepo(engine),
		Printers: repositories.NewPrinterRepo(engine),
		Articles: repositories.NewArticleRepo(engine),
		Tmpl:     tmpl,
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
		Feeds:    repositories.NewFeedRepo(engine),
		Printers: repositories.NewPrinterRepo(engine),
		Articles: repositories.NewArticleRepo(engine),
		Tmpl:     tmpl,
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
