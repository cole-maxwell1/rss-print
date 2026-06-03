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

func renderPage(w http.ResponseWriter, r *http.Request, tmpl *template.Template, data map[string]any) error {
	if r.Header.Get("HX-Request") == "true" {
		if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
			return err
		}
		return tmpl.ExecuteTemplate(w, "nav", data)
	}
	return tmpl.ExecuteTemplate(w, "base.html", data)
}
