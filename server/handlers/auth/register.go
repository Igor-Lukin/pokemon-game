package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

func InitHandlers(database *sql.DB) {
	db = database
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./server/templates/register.html")
}

func RegisterPostHandler(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Username        string `json:"username"`
		Email           string `json:"email"`
		Password        string `json:"password"`
		ConfirmPassword string `json:"confirm_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid data format", http.StatusBadRequest)
		return
	}

	if data.Password != data.ConfirmPassword {
		http.Error(w, "Passwords don't match", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec(
		"INSERT INTO users (username, email, password) VALUES (?, ?, ?)",
		data.Username, data.Email, string(hash),
	)

	if err != nil {
		http.Error(w, "Registration failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
