package middleware

import (
	"context"
	"net/http"

	"rss-print/internal/models"
	"rss-print/internal/repositories"

	"github.com/gorilla/sessions"
)

var Store = sessions.NewCookieStore([]byte("rss-print-secret-key-change-me")) // TODO: move to env

type contextKey string

const UserContextKey contextKey = "user"

// AuthMiddleware ensures the user is logged in
func AuthMiddleware(users *repositories.UserRepo, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := Store.Get(r, "session-name")
		userID, ok := session.Values["userID"].(int64)

		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		user, has, err := users.GetByID(userID)
		if err != nil || !has {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetUser returns the user from the context
func GetUser(ctx context.Context) *models.User {
	user, ok := ctx.Value(UserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}
