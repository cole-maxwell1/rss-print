package services

import (
	"context"
	"net/http"

	"github.com/mmcdole/gofeed"
)

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
