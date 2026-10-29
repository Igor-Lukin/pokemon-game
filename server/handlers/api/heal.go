package api

import (
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
)

func HealHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var hasHealing int
	err := db.DB.QueryRow(`
		SELECT COALESCE(l.has_healing, 0)
		FROM users u
		LEFT JOIN locations l ON l.id = u.location_id
		WHERE u.id = ?`, userID).Scan(&hasHealing)
	if err != nil {
		log.Printf("HealHandler: location lookup error: %v", err)
		http.Error(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	if hasHealing != 1 {
		http.Error(w, "Здесь нет Покецентра", http.StatusBadRequest)
		return
	}

	// Restore HP to max for all user's pokemon
	_, err = db.DB.Exec("UPDATE user_pokemons SET current_hp = max_hp, status_id = 1 WHERE user_id = ?", userID)
	if err != nil {
		log.Printf("HealHandler: HP update error: %v", err)
		http.Error(w, "Failed to heal", http.StatusInternalServerError)
		return
	}

	// Restore PP for all learned moves
	_, err = db.DB.Exec(`
		UPDATE user_pokemon_learnable_attacks
		SET current_pp = (SELECT pp FROM attacks WHERE attacks.id = attack_id)
		WHERE user_pokemon_id IN (SELECT id FROM user_pokemons WHERE user_id = ?)
		  AND is_learned = 1
	`, userID)
	if err != nil {
		log.Printf("HealHandler: PP update error: %v", err)
		http.Error(w, "Failed to restore PP", http.StatusInternalServerError)
		return
	}

	log.Printf("User %d healed all pokemon", userID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
