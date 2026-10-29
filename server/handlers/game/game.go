package game

import (
	"html/template"
	"net/http"
	"pokemon-game/server/session"
)

func GameHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	_, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Получаем username из сессии (можно также из базы, но в сессии он уже сохранён при логине)
	username, _ := sess.Values["username"].(string)
	if username == "" {
		// Если по какой-то причине username нет в сессии, можно запросить из БД
		// Но для простоты оставим заглушку
		username = "Игрок"
	}

	tmpl := template.Must(template.ParseFiles("./server/templates/game.html"))
	tmpl.Execute(w, struct {
		Username string
		Version  string
	}{Username: username, Version: "46"})
}

func GamePostHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	if _, ok := sess.Values["user_id"]; !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	http.ServeFile(w, r, "./server/templates/game.html")
}
