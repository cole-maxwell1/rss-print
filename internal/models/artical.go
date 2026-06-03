package models

import "time"

// Article represents a parsed RSS item, used to prevent duplicate printing
type Article struct {
	ID          int64     `xorm:"pk autoincr 'id'"`
	FeedID      int64     `xorm:"notnull index 'feed_id'"`
	GUID        string    `xorm:"notnull unique(feed_guid) 'guid'"` // Prevent duplicates per feed
	Title       string    `xorm:"notnull 'title'"`
	URL         string    `xorm:"'url'"`
	PublishedAt time.Time `xorm:"'published_at'"`
	CreatedAt   time.Time `xorm:"created 'created_at'"`
}
