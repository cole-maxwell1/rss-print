package repositories

import (
	"rss-print/internal/models"

	"xorm.io/xorm"
)

// ArticleRepo provides data access for Article records.
type ArticleRepo struct {
	engine *xorm.Engine
}

func NewArticleRepo(engine *xorm.Engine) *ArticleRepo {
	return &ArticleRepo{engine: engine}
}

// GetByID returns the article with the given id, or found=false when none exists.
func (r *ArticleRepo) GetByID(id int64) (*models.Article, bool, error) {
	var article models.Article
	has, err := r.engine.ID(id).Get(&article)
	if err != nil || !has {
		return nil, has, err
	}
	return &article, true, nil
}

// Create inserts a new article. The (feed_id, guid) unique constraint causes a
// duplicate insert to return an error, which callers treat as a skip signal.
func (r *ArticleRepo) Create(article *models.Article) error {
	_, err := r.engine.Insert(article)
	return err
}

// ListRecent returns the most recent articles, newest first, up to limit.
func (r *ArticleRepo) ListRecent(limit int) ([]models.Article, error) {
	var articles []models.Article
	if err := r.engine.OrderBy("created_at DESC").Limit(limit).Find(&articles); err != nil {
		return nil, err
	}
	return articles, nil
}
