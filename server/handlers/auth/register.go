package auth

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,20}$`)

// isUniqueViolation — SQLite-ошибка нарушения UNIQUE (username/email).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

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
		http.Error(w, "Некорректные данные", http.StatusBadRequest)
		return
	}

	data.Username = strings.TrimSpace(data.Username)
	data.Email = strings.TrimSpace(data.Email)

	if !usernameRe.MatchString(data.Username) {
		http.Error(w, "Имя пользователя: 3–20 символов, только латиница, цифры и _", http.StatusBadRequest)
		return
	}
	if len(data.Email) > 100 || !strings.Contains(data.Email, "@") || !strings.Contains(data.Email, ".") {
		http.Error(w, "Некорректный email", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(data.Password) < 8 || len(data.Password) > 72 {
		http.Error(w, "Пароль должен быть от 8 до 72 символов", http.StatusBadRequest)
		return
	}

	if data.Password != data.ConfirmPassword {
		http.Error(w, "Пароли не совпадают", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec(
		`INSERT INTO users (username, email, password, location_id)
		 VALUES (?, ?, ?, COALESCE(
			(SELECT id FROM locations WHERE id = 300),
			(SELECT MIN(id) FROM locations)
		 ))`,
		data.Username, data.Email, string(hash),
	)

	if err != nil {
		log.Printf("register insert: %v", err)
		if isUniqueViolation(err) {
			http.Error(w, "Пользователь с таким именем уже существует", http.StatusConflict)
			return
		}
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
