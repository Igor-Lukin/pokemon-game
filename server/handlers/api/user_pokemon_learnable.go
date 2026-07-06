package api

import (
	"encoding/json"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"

	"github.com/gorilla/mux"
)

type LearnableAttack struct {
	ID            int    `json:"id"`
	AttackID      int    `json:"attack_id"`
	TypeID        int    `json:"type_id"`
	Name          string `json:"name"`
	CategoryName  string `json:"category_name"`
	LevelAvailable int   `json:"level_available"`
	IsLearned     bool   `json:"is_learned"`
}

func GetLearnableAttacksHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userPokemonID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ownerID int
	err = db.DB.QueryRow("SELECT user_id FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	rows, err := db.DB.Query(`
		SELECT ula.id, ula.attack_id, a.type_id, a.name,
		       CASE a.category_id
		           WHEN 1 THEN 'Физическая'
		           WHEN 2 THEN 'Специальная'
		           WHEN 3 THEN 'Статусовая'
		       END,
		       ula.level_available, ula.is_learned
		FROM user_pokemon_learnable_attacks ula
		JOIN attacks a ON ula.attack_id = a.id
		WHERE ula.user_pokemon_id = ?
		ORDER BY ula.level_available, a.name
	`, userPokemonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := make([]LearnableAttack, 0)
	for rows.Next() {
		var la LearnableAttack
		if err := rows.Scan(&la.ID, &la.AttackID, &la.TypeID, &la.Name, &la.CategoryName, &la.LevelAvailable, &la.IsLearned); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		list = append(list, la)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

type LearnRequest struct {
	LearnableID     int `json:"learnable_id"`
	ReplacePosition int `json:"replace_position"`
}

func LearnAttackHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userPokemonID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req LearnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var ownerID int
	err = db.DB.QueryRow("SELECT user_id FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Убеждаемся, что learnable запись принадлежит этому покемону и ещё не выучена
	var currentIsLearned bool
	err = db.DB.QueryRow("SELECT is_learned FROM user_pokemon_learnable_attacks WHERE id = ? AND user_pokemon_id = ?",
		req.LearnableID, userPokemonID).Scan(&currentIsLearned)
	if err != nil {
		http.Error(w, "Learnable attack not found", http.StatusNotFound)
		return
	}
	if currentIsLearned {
		http.Error(w, "Attack already learned", http.StatusConflict)
		return
	}

	// Если есть атака на этой позиции — сбрасываем её
	if req.ReplacePosition >= 1 && req.ReplacePosition <= 4 {
		_, err = db.DB.Exec(`
			UPDATE user_pokemon_learnable_attacks
			SET position = NULL, is_learned = 0
			WHERE user_pokemon_id = ? AND position = ? AND is_learned = 1
		`, userPokemonID, req.ReplacePosition)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Изучаем выбранную атаку
	_, err = db.DB.Exec(`
		UPDATE user_pokemon_learnable_attacks
		SET is_learned = 1, position = ?,
		    current_pp = (SELECT pp FROM attacks WHERE id = (SELECT attack_id FROM user_pokemon_learnable_attacks WHERE id = ?))
		WHERE id = ? AND user_pokemon_id = ?
	`, req.ReplacePosition, req.LearnableID, req.LearnableID, userPokemonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
