package services

import (
	"context"
	"net/http"
	"strings"

	"github.com/mmcdole/gofeed"
)

// FeedItemContent returns the richest body a feed item carries: the full content
// when present, otherwise the description/summary. May be empty.
func FeedItemContent(item *gofeed.Item) string {
	if strings.TrimSpace(item.Content) != "" {
		return item.Content
	}
	return item.Description
}

// FetchFeed downloads and parses an RSS or Atom feed
func FetchFeed(ctx context.Context, url string, headerName string, headerValue string) (*gofeed.Feed, error) {
	fp := gofeed.NewParser()
	if headerName != "" {
		fp.Client = &http.Client{
			Transport: headerTransport{
				base:        http.DefaultTransport,
				headerName:  headerName,
				headerValue: headerValue,
			},
		}
	}
	return fp.ParseURLWithContext(url, ctx)
}

type headerTransport struct {
	base        http.RoundTripper
	headerName  string
	headerValue string
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set(t.headerName, t.headerValue)
	return t.base.RoundTrip(clone)
}
