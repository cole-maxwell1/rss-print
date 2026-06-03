package repositories

import (
	"rss-print/internal/models"

	"xorm.io/xorm"
)

// FeedRepo provides data access for Feed records.
type FeedRepo struct {
	engine *xorm.Engine
}

func NewFeedRepo(engine *xorm.Engine) *FeedRepo {
	return &FeedRepo{engine: engine}
}

// List returns all feeds, newest first.
func (r *FeedRepo) List() ([]models.Feed, error) {
	var feeds []models.Feed
	if err := r.engine.OrderBy("created_at DESC").Find(&feeds); err != nil {
		return nil, err
	}
	return feeds, nil
}

// Create inserts a new feed.
func (r *FeedRepo) Create(feed *models.Feed) error {
	_, err := r.engine.Insert(feed)
	return err
}

// UpdatePollResult persists the outcome of a poll: last error and poll time.
func (r *FeedRepo) UpdatePollResult(feed *models.Feed) error {
	_, err := r.engine.ID(feed.ID).Cols("last_error", "last_polled_at").Update(feed)
	return err
}
