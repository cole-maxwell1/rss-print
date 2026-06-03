package handlers

import "rss-print/internal/models"

// pageData holds the fields shared by every authenticated page payload. It is
// embedded into the per-page structs so templates resolve .User, .Path, .Notice
// and .Error via field promotion.
type pageData struct {
	User   *models.User
	Path   string
	Notice string
	Error  string
}
