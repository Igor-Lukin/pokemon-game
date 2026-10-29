package api

import (
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"

	"github.com/gorilla/mux"
)

// SetStarterHandler назначает стартового покемона команды. Стартовый всегда
// выбирается первым в диком/тренерском бою, а в PvP показывается первым в списке
// выбора лида.
func SetStarterHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pokemonID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "Invalid pokemon id", http.StatusBadRequest)
		return
	}

	var id int
	err = db.DB.QueryRow(`
		SELECT id FROM user_pokemons
		WHERE id = ? AND user_id = ? AND position BETWEEN 1 AND 6`,
		pokemonID, userID).Scan(&id)
	if err != nil {
		http.Error(w, "Pokemon not found in team", http.StatusNotFound)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Стартовый может быть только один — снимаем флаг у остальных.
	if _, err = tx.Exec("UPDATE user_pokemons SET is_starter = 0 WHERE user_id = ?", userID); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if _, err = tx.Exec("UPDATE user_pokemons SET is_starter = 1 WHERE id = ? AND user_id = ?", pokemonID, userID); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}
