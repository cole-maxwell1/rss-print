package handlers

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"rss-print/internal/models"
)

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
