package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"rss-print/internal/models"
	"rss-print/internal/repositories"
	"rss-print/internal/services"
)

// Worker handles background tasks like polling feeds and dispatching prints
type Worker struct {
	Feeds    *repositories.FeedRepo
	Articles *repositories.ArticleRepo
	Printers *repositories.PrinterRepo
	Jobs     *repositories.PrintJobRepo
	wg       sync.WaitGroup
}

// Start begins the background processing loops
func (w *Worker) Start(ctx context.Context) {
	w.wg.Go(func() {
		ticker := time.NewTicker(30 * time.Second) // poll faster for jobs
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("Stopping Worker loops...")
				return
			case <-ticker.C:
				w.pollFeeds()
				w.processJobs()
			}
		}
	})
}

// Wait blocks until the background loop has exited. Call after cancelling the
// context passed to Start so the worker stops writing before the DB closes.
func (w *Worker) Wait() {
	w.wg.Wait()
}

func (w *Worker) pollFeeds() {
	feeds, err := w.Feeds.List()
	if err != nil {
		log.Printf("Worker failed to fetch feeds: %v", err)
		return
	}

	now := time.Now()
	for _, feed := range feeds {
		nextPoll := feed.LastPolledAt.Add(time.Duration(feed.PollInterval) * time.Minute)
		if now.After(nextPoll) {
			w.processFeed(feed)
		}
	}
}

func (w *Worker) processFeed(feed models.Feed) {
	log.Printf("Worker polling feed: %s", feed.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	parsedFeed, err := services.FetchFeed(ctx, feed.URL, feed.AuthHeaderName, feed.AuthHeaderValue)
	if err != nil {
		log.Printf("Worker failed to parse feed %s: %v", feed.URL, err)
		feed.LastError = err.Error()
		feed.LastPolledAt = time.Now()
		w.Feeds.UpdatePollResult(&feed)
		return
	}

	feed.LastError = ""
	feed.LastPolledAt = time.Now()
	w.Feeds.UpdatePollResult(&feed)

	for _, item := range parsedFeed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link // Fallback to URL if no GUID
		}

		article := &models.Article{
			FeedID:  feed.ID,
			GUID:    guid,
			Title:   item.Title,
			URL:     item.Link,
			Content: services.FeedItemContent(item),
		}

		if item.PublishedParsed != nil {
			article.PublishedAt = *item.PublishedParsed
		}

		err := w.Articles.Create(article)
		if err != nil {
			// Expected error for duplicates. Skip job creation.
			continue
		}

		log.Printf("New article found: %s", article.Title)

		if feed.AutoPrint {
			job := &models.PrintJob{
				ArticleID: article.ID,
				PrinterID: feed.PrinterID, // Can be 0 for default
				Status:    "Pending",
			}
			err = w.Jobs.Create(job)
			if err != nil {
				log.Printf("Worker failed to create print job: %v", err)
			} else {
				log.Printf("Worker queued print job for article: %s", article.Title)
			}
		}
	}
}

func (w *Worker) processJobs() {
	// Get Pending jobs, or Failed jobs that haven't exceeded retry count
	jobs, err := w.Jobs.ListPending()
	if err != nil {
		log.Printf("Worker failed to fetch jobs: %v", err)
		return
	}

	for _, job := range jobs {
		w.executeJob(&job)
	}
}

func (w *Worker) executeJob(job *models.PrintJob) {
	log.Printf("Executing print job %d", job.ID)

	article, has, err := w.Articles.GetByID(job.ArticleID)
	if err != nil || !has {
		log.Printf("Article not found for job %d", job.ID)
		w.markJobFailed(job, "Article not found")
		return
	}

	printer, has, err := w.Printers.GetByID(job.PrinterID)
	if err != nil || !has {
		// Try to fallback to global default printer
		printer, has, err = w.Printers.GetDefault()
		if err != nil || !has {
			log.Printf("No printer found for job %d", job.ID)
			w.markJobFailed(job, "No printer configured or default printer found")
			return
		}
	}

	// 1. Generate PDF
	pdfBytes, err := services.BuildArticlePDF(context.Background(), article.Title, article.URL, article.Content, func(content string) {
		if uerr := w.Articles.UpdateContent(article.ID, content); uerr != nil {
			log.Printf("Failed to backfill content for article %d: %v", article.ID, uerr)
		}
	})
	if err != nil {
		log.Printf("Failed to generate PDF for job %d: %v", job.ID, err)
		w.markJobFailed(job, err.Error())
		return
	}

	// 2. Print PDF
	err = services.PrintPDF(printer.URI, pdfBytes, "Article: "+article.Title)
	if err != nil {
		log.Printf("Failed to print job %d: %v", job.ID, err)
		w.markJobFailed(job, err.Error())
		return
	}

	// Success
	job.Status = "Sent"
	job.LastError = ""
	w.Jobs.UpdateStatus(job)
	log.Printf("Job %d completed successfully", job.ID)
}

func (w *Worker) markJobFailed(job *models.PrintJob, reason string) {
	job.Status = "Failed"
	job.LastError = reason
	job.RetryCount++
	w.Jobs.MarkFailed(job)
}
