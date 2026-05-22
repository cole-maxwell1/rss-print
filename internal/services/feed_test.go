package services

import (
	"context"
	"net/http"
	"testing"
)

func TestHeaderTransportSendsCustomHeader(t *testing.T) {
	transport := headerTransport{
		base: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("X-Token"); got != "secret" {
				t.Fatalf("expected custom header, got %q", got)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		headerName:  "X-Token",
		headerValue: "secret",
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com/feed.xml", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}
	if got := req.Header.Get("X-Token"); got != "" {
		t.Fatalf("transport should not mutate original request, got original header %q", got)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
