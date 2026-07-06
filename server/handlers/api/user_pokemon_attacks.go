package api

import (
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"

	"github.com/gorilla/mux"
)

type UserPokemonAttack struct {
	Name       string `json:"name"`
	TypeID     int    `json:"type_id"`
	CategoryID int    `json:"category_id"`
	PP         int    `json:"pp"`
	MaxPP      int    `json:"max_pp"`
}

func GetUserPokemonAttacks(w http.ResponseWriter, r *http.Request) {
	// Получаем ID из URL
	vars := mux.Vars(r)
	userPokemonIDStr := vars["id"]
	userPokemonID, err := strconv.Atoi(userPokemonIDStr)
	if err != nil {
		http.Error(w, "Invalid user pokemon id", http.StatusBadRequest)
		return
	}

	// Проверяем, принадлежит ли покемон текущему пользователю
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var ownerID int
	var pokemonID int
	err = db.DB.QueryRow("SELECT user_id, pokemon_id FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&ownerID, &pokemonID)
	if err != nil || ownerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Загружаем атаки для этого вида покемона (из pokemon_attacks)
	rows, err := db.DB.Query(`
        SELECT a.name, a.type_id, a.category_id, a.pp, a.pp
        FROM pokemon_attacks pa
        JOIN attacks a ON pa.attack_id = a.id
        WHERE pa.pokemon_id = ?
        ORDER BY pa.learn_method_id, pa.level_learned_at
    `, pokemonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var attacks []UserPokemonAttack
	for rows.Next() {
		var atk UserPokemonAttack
		if err := rows.Scan(&atk.Name, &atk.TypeID, &atk.CategoryID, &atk.PP, &atk.MaxPP); err != nil {
			continue
		}
		attacks = append(attacks, atk)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(attacks)
}
