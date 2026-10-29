package auth

import (
	"log"
	"net/http"
	"pokemon-game/server/session"

	"golang.org/x/crypto/bcrypt"
)

// LoginHandler отображает страницу входа (GET)
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./server/templates/login.html")
}

// LoginPostHandler обрабатывает отправку формы входа (POST)
func LoginPostHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	log.Println("Login attempt:", username)

	var user struct {
		ID         int
		Username   string
		Email      string
		LocationID int
		Password   string
	}
	err := db.QueryRow("SELECT id, username, email, location_id, password FROM users WHERE username = ?", username).
		Scan(&user.ID, &user.Username, &user.Email, &user.LocationID, &user.Password)

	if err != nil {
		log.Println("Auth error:", err)
		http.Error(w, "Неверные данные", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		http.Error(w, "Неверные данные", http.StatusUnauthorized)
		return
	}

	log.Println("User found:", user.ID, user.Username)

	sess, _ := session.Store.Get(r, "pokemon-session")
	sess.Values["user_id"] = user.ID
	sess.Values["username"] = user.Username
	sess.Save(r, w)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
