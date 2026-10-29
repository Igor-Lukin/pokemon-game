package session

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/sessions"
)

var Store *sessions.CookieStore

func Init() {
	key := os.Getenv("SESSION_KEY")
	if key == "" {
		log.Fatal("SESSION_KEY environment variable is not set")
	}
	Store = sessions.NewCookieStore([]byte(key))
	Store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Secure включать только после перехода на https: на http
		// браузер перестанет присылать cookie.
	}
}
