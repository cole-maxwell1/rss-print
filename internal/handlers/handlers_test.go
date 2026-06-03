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

	"rss-print/internal/db"
	"rss-print/internal/middleware"
	"rss-print/internal/models"
	"rss-print/ui"

	"xorm.io/xorm"
)

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
