package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

func parseOptionalInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

// pdfFilename turns an article title into a safe, slugified PDF filename.
// Non-alphanumeric runs collapse to single dashes; an empty result falls back
// to "article".
func pdfFilename(title string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "article"
	}
	return slug + ".pdf"
}

func renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, data any) error {
	if r.Header.Get("HX-Request") == "true" {
		if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
			return err
		}
		return tmpl.ExecuteTemplate(w, "nav", data)
	}
	return tmpl.ExecuteTemplate(w, "base.html", data)
}
