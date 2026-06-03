package handlers

import (
	"html/template"
	"log"
	"net/http"

	"rss-print/internal/middleware"
	"rss-print/internal/models"

	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"
)

type AuthHandler struct {
	DB   *xorm.Engine
	Tmpl *template.Template
}

func (h *AuthHandler) RenderLogin(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{}
	if r.URL.Query().Get("error") == "1" {
		data["Error"] = "Invalid username or password"
	}
	if err := h.Tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("failed to render login template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user := &models.User{}
	has, err := h.DB.Where("username = ?", username).Get(user)
	if err != nil || !has {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}

	session, _ := middleware.Store.Get(r, "session-name")
	session.Values["userID"] = user.ID
	session.Save(r, w)

	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := middleware.Store.Get(r, "session-name")
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// CreateDefaultUser creates a default admin user if no users exist
func CreateDefaultUser(db *xorm.Engine) error {
	count, err := db.Count(new(models.User))
	if err != nil {
		return err
	}
	if count == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		user := &models.User{
			Username:     "admin",
			PasswordHash: string(hash),
		}
		_, err = db.Insert(user)
		if err != nil {
			return err
		}
		log.Println("Created default user: admin / admin")
	}
	return nil
}
