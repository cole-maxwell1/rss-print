package handlers

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"rss-print/internal/models"
	"rss-print/internal/repositories"
)

func TestPrinterCreateUpsertsByURI(t *testing.T) {
	engine := testDB(t)
	handler := &PrinterHandler{Printers: repositories.NewPrinterRepo(engine), Tmpl: testTemplate(t, "templates/printers.html")}

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
	handler := &PrinterHandler{Printers: repositories.NewPrinterRepo(engine), Tmpl: testTemplate(t, "templates/printers.html")}

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
