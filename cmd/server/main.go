package main

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rss-print/internal/db"
	"rss-print/internal/handlers"
	"rss-print/internal/middleware"
	"rss-print/internal/worker"
	"rss-print/ui"
)

func main() {
	log.Println("Starting RSS Auto-Print Server...")

	// Cancel the root context on interrupt or termination so the worker loop
	// and HTTP server can shut down cleanly before the database is closed.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Initialize Database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "rss-print.db" // Default fallback
	}

	engine, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Fatal error initializing database: %v", err)
	}

	// Ensure at least one admin user exists
	if err := handlers.CreateDefaultUser(engine); err != nil {
		log.Fatalf("Failed to create default user: %v", err)
	}

	// 2. Start Background Worker
	bgWorker := &worker.Worker{DB: engine}
	bgWorker.Start(ctx)

	// 3. Parse Templates
	loginTmpl := template.Must(template.ParseFS(ui.FS, "templates/base.html", "templates/login.html"))
	dashTmpl := template.Must(template.ParseFS(ui.FS, "templates/base.html", "templates/dashboard.html"))
	feedsTmpl := template.Must(template.ParseFS(ui.FS, "templates/base.html", "templates/feeds.html"))
	printersTmpl := template.Must(template.ParseFS(ui.FS, "templates/base.html", "templates/printers.html"))

	// 4. Setup HTTP Router
	mux := http.NewServeMux()

	// Serve Static Files
	mux.Handle("GET /static/", http.FileServer(http.FS(ui.FS)))

	// Handlers
	authH := &handlers.AuthHandler{DB: engine, Tmpl: loginTmpl}
	dashH := &handlers.DashboardHandler{DB: engine, Tmpl: dashTmpl}
	feedH := &handlers.FeedHandler{DB: engine, Tmpl: feedsTmpl}
	printerH := &handlers.PrinterHandler{DB: engine, Tmpl: printersTmpl}

	// Public Routes
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("GET /login", authH.RenderLogin)
	mux.HandleFunc("POST /login", authH.HandleLogin)
	mux.HandleFunc("POST /logout", authH.HandleLogout)

	// Protected Routes
	mux.HandleFunc("GET /", middleware.AuthMiddleware(engine, dashH.Render))
	mux.HandleFunc("POST /prints", middleware.AuthMiddleware(engine, dashH.HandleCreatePrint))
	mux.HandleFunc("POST /prints/{id}/retry", middleware.AuthMiddleware(engine, dashH.HandleRetryPrint))
	mux.HandleFunc("GET /feed", middleware.AuthMiddleware(engine, feedH.Render))
	mux.HandleFunc("GET /feeds", middleware.AuthMiddleware(engine, feedH.Render))
	mux.HandleFunc("POST /feeds", middleware.AuthMiddleware(engine, feedH.HandleCreate))
	mux.HandleFunc("GET /printers", middleware.AuthMiddleware(engine, printerH.Render))
	mux.HandleFunc("POST /printers/discover", middleware.AuthMiddleware(engine, printerH.HandleDiscover))
	mux.HandleFunc("POST /printers", middleware.AuthMiddleware(engine, printerH.HandleCreate))
	mux.HandleFunc("POST /printers/{id}/default", middleware.AuthMiddleware(engine, printerH.HandleSetDefault))

	// 5. Start Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	go func() {
		log.Printf("Server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Server failed: %v", err)
			stop() // unblock main and trigger shutdown
		}
	}()

	// 6. Graceful shutdown: stop serving requests, stop the worker, then close
	// the database so the WAL is checkpointed instead of left active.
	<-ctx.Done()
	log.Println("Shutdown signal received, stopping server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	bgWorker.Wait()

	if err := db.Close(engine); err != nil {
		log.Printf("Database close error: %v", err)
	}
	log.Println("Shutdown complete.")
}
