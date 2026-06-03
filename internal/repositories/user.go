package repositories

import (
	"rss-print/internal/models"

	"xorm.io/xorm"
)

// UserRepo provides data access for User records.
type UserRepo struct {
	engine *xorm.Engine
}

func NewUserRepo(engine *xorm.Engine) *UserRepo {
	return &UserRepo{engine: engine}
}

// GetByUsername returns the user matching username, or found=false when none exists.
func (r *UserRepo) GetByUsername(username string) (*models.User, bool, error) {
	var user models.User
	has, err := r.engine.Where("username = ?", username).Get(&user)
	if err != nil || !has {
		return nil, has, err
	}
	return &user, true, nil
}

// GetByID returns the user with the given id, or found=false when none exists.
func (r *UserRepo) GetByID(id int64) (*models.User, bool, error) {
	var user models.User
	has, err := r.engine.ID(id).Get(&user)
	if err != nil || !has {
		return nil, has, err
	}
	return &user, true, nil
}

// Count returns the total number of users.
func (r *UserRepo) Count() (int64, error) {
	return r.engine.Count(new(models.User))
}

// Create inserts a new user.
func (r *UserRepo) Create(user *models.User) error {
	_, err := r.engine.Insert(user)
	return err
}
