package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"

	"github.com/gorilla/mux"
)

type LearnableAttack struct {
	ID             int    `json:"id"`
	AttackID       int    `json:"attack_id"`
	TypeID         int    `json:"type_id"`
	Name           string `json:"name"`
	CategoryName   string `json:"category_name"`
	LevelAvailable int    `json:"level_available"`
	IsLearned      bool   `json:"is_learned"`
	Source         string `json:"source"` // level | tm | hm
	SourceItemID   *int   `json:"source_item_id,omitempty"`
	SourceItemName string `json:"source_item_name,omitempty"`
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
		       ula.level_available, ula.is_learned,
		       COALESCE(ula.source, 'level'), ula.source_item_id,
		       COALESCE(i.name, '')
		FROM user_pokemon_learnable_attacks ula
		JOIN attacks a ON ula.attack_id = a.id
		LEFT JOIN items i ON i.id = ula.source_item_id
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
		var sourceItemID sql.NullInt64
		if err := rows.Scan(&la.ID, &la.AttackID, &la.TypeID, &la.Name, &la.CategoryName,
			&la.LevelAvailable, &la.IsLearned, &la.Source, &sourceItemID, &la.SourceItemName); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		if sourceItemID.Valid {
			id := int(sourceItemID.Int64)
			la.SourceItemID = &id
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

	replacePosition := req.ReplacePosition
	if replacePosition < 1 || replacePosition > 4 {
		replacePosition = firstFreeMovePosition(userPokemonID)
		if replacePosition == 0 {
			http.Error(w, "Все 4 слота атак заняты — выберите, какую заменить", http.StatusConflict)
			return
		}
	}

	// Если есть атака на этой позиции — сбрасываем её. Атаки, открытые TM/HM,
	// при замене удаляются из списка (нужен новый предмет), если они не
	// доступны этому виду по уровню.
	var oldAttackID int
	var oldSource string
	err = db.DB.QueryRow(`
		SELECT attack_id, COALESCE(source, 'level')
		FROM user_pokemon_learnable_attacks
		WHERE user_pokemon_id = ? AND position = ? AND is_learned = 1
	`, userPokemonID, replacePosition).Scan(&oldAttackID, &oldSource)
	if err == nil {
		if (oldSource == "tm" || oldSource == "hm") && !levelMoveAvailable(userPokemonID, oldAttackID) {
			_, err = db.DB.Exec(`
				DELETE FROM user_pokemon_learnable_attacks
				WHERE user_pokemon_id = ? AND position = ? AND is_learned = 1
			`, userPokemonID, replacePosition)
		} else if oldSource == "tm" || oldSource == "hm" {
			_, err = db.DB.Exec(`
				UPDATE user_pokemon_learnable_attacks
				SET position = NULL, is_learned = 0, source = 'level', source_item_id = NULL
				WHERE user_pokemon_id = ? AND position = ? AND is_learned = 1
			`, userPokemonID, replacePosition)
		} else {
			_, err = db.DB.Exec(`
				UPDATE user_pokemon_learnable_attacks
				SET position = NULL, is_learned = 0
				WHERE user_pokemon_id = ? AND position = ? AND is_learned = 1
			`, userPokemonID, replacePosition)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Изучаем выбранную атаку
	_, err = db.DB.Exec(`
		UPDATE user_pokemon_learnable_attacks
		SET is_learned = 1, position = ?,
		    current_pp = (SELECT pp FROM attacks WHERE id = (SELECT attack_id FROM user_pokemon_learnable_attacks WHERE id = ?))
		WHERE id = ? AND user_pokemon_id = ?
	`, replacePosition, req.LearnableID, req.LearnableID, userPokemonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// firstFreeMovePosition возвращает первый свободный слот 1..4 или 0.
func firstFreeMovePosition(userPokemonID int) int {
	for pos := 1; pos <= 4; pos++ {
		var n int
		err := db.DB.QueryRow(`
			SELECT COUNT(*) FROM user_pokemon_learnable_attacks
			WHERE user_pokemon_id = ? AND position = ? AND is_learned = 1
		`, userPokemonID, pos).Scan(&n)
		if err == nil && n == 0 {
			return pos
		}
	}
	return 0
}

// levelMoveAvailable — доступна ли атака этому покемону по уровню (level-up).
func levelMoveAvailable(userPokemonID, attackID int) bool {
	var one int
	err := db.DB.QueryRow(`
		SELECT 1
		FROM user_pokemons up
		JOIN pokemon_attacks pa ON pa.pokemon_id = up.pokemon_id
		WHERE up.id = ? AND pa.attack_id = ? AND pa.learn_method_id = 1
		  AND pa.level_learned_at <= up.level
		LIMIT 1
	`, userPokemonID, attackID).Scan(&one)
	return err == nil
}
