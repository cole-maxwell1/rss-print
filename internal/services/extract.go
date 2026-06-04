package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

// minFeedContentLen is the plain-text length, in characters, above which the
// feed's own content is considered a full article rather than a truncated
// summary. Below it, the live page is fetched and extracted instead.
const minFeedContentLen = 600

// ExtractArticleHTML returns cleaned article HTML suitable for PDF rendering.
//
// It is hybrid: when fallbackHTML (the feed's content/description) already holds
// a substantial body, it is used directly. Otherwise the live article URL is
// fetched and run through readability extraction. On any extraction failure it
// degrades to whatever fallbackHTML was provided so a PDF can still be produced.
func ExtractArticleHTML(ctx context.Context, articleURL, fallbackHTML string) (cleanHTML, baseURL string, err error) {
	baseURL = strings.TrimSpace(articleURL)

	if len(strings.TrimSpace(stripHTML(fallbackHTML))) >= minFeedContentLen {
		return fallbackHTML, baseURL, nil
	}

	if baseURL == "" {
		if strings.TrimSpace(fallbackHTML) != "" {
			return fallbackHTML, baseURL, nil
		}
		return "", baseURL, fmt.Errorf("no article url or content to extract")
	}

	parsed, perr := url.Parse(baseURL)
	if perr != nil {
		return fallbackHTML, baseURL, fmt.Errorf("parse article url: %w", perr)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, rerr := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL, nil)
	if rerr != nil {
		return fallbackHTML, baseURL, rerr
	}
	req.Header.Set("User-Agent", pdfUserAgent)

	resp, derr := http.DefaultClient.Do(req)
	if derr != nil {
		return fallbackHTML, baseURL, fmt.Errorf("fetch article: %w", derr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallbackHTML, baseURL, fmt.Errorf("fetch article %s: status %d", baseURL, resp.StatusCode)
	}

	article, aerr := readability.FromReader(resp.Body, parsed)
	if aerr != nil {
		return fallbackHTML, baseURL, fmt.Errorf("extract article: %w", aerr)
	}
	if strings.TrimSpace(article.Content) == "" {
		return fallbackHTML, baseURL, nil
	}
	return article.Content, baseURL, nil
}

// stripHTML returns the concatenated text content of an HTML fragment, dropping
// all tags. On a tokenizer error it returns the original input unchanged.
func stripHTML(htmlStr string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(htmlStr))
	var text strings.Builder

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			if tokenizer.Err() == io.EOF {
				return text.String()
			}
			return htmlStr // fallback
		}

		if tt == html.TextToken {
			text.WriteString(string(tokenizer.Text()))
		}
	}
}
