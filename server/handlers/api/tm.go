package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"

	"github.com/gorilla/mux"
)

type TMTarget struct {
	ID        int    `json:"id"`
	PokemonID int    `json:"pokemon_id"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
	CurrentHP int    `json:"current_hp"`
	MaxHP     int    `json:"max_hp"`
	Position  int    `json:"position"`
	CanLearn  bool   `json:"can_learn"`
	Reason    string `json:"reason,omitempty"`
}

type TMTargetsResponse struct {
	ItemID     int        `json:"item_id"`
	ItemName   string     `json:"item_name"`
	EffectType string     `json:"effect_type"`
	AttackID   int        `json:"attack_id"`
	AttackName string     `json:"attack_name"`
	Targets    []TMTarget `json:"targets"`
}

// GetTMTargetsHandler — покемоны в команде и совместимость с конкретным TM/HM.
func GetTMTargetsHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	itemID, err := strconv.Atoi(mux.Vars(r)["item_id"])
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}
	item, err := db.GetItemByID(itemID)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if item.AttackID <= 0 || (item.EffectType != "tm" && item.EffectType != "hm") {
		http.Error(w, "Это не TM/HM-предмет", http.StatusBadRequest)
		return
	}

	var attackName string
	err = db.DB.QueryRow("SELECT name FROM attacks WHERE id = ? AND COALESCE(disabled, 0) = 0", item.AttackID).Scan(&attackName)
	if err == sql.ErrNoRows {
		http.Error(w, "Атака не найдена или недоступна", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := db.DB.Query(`
		SELECT up.id, up.pokemon_id, p.name, up.level, up.current_hp, up.max_hp, up.position,
		       EXISTS(
		           SELECT 1 FROM pokemon_attacks pa
		           WHERE pa.pokemon_id = up.pokemon_id AND pa.attack_id = ? AND pa.learn_method_id = 2
		       ) AS compatible,
		       EXISTS(
		           SELECT 1 FROM user_pokemon_learnable_attacks ula
		           WHERE ula.user_pokemon_id = up.id AND ula.attack_id = ?
		       ) AS in_list
		FROM user_pokemons up
		JOIN pokemons p ON p.id = up.pokemon_id
		WHERE up.user_id = ? AND up.position BETWEEN 1 AND 6
		ORDER BY up.position
	`, item.AttackID, item.AttackID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	targets := make([]TMTarget, 0)
	for rows.Next() {
		var t TMTarget
		var compatible, inList bool
		if err := rows.Scan(&t.ID, &t.PokemonID, &t.Name, &t.Level, &t.CurrentHP, &t.MaxHP,
			&t.Position, &compatible, &inList); err != nil {
			continue
		}
		switch {
		case !compatible:
			t.Reason = "Не может выучить эту атаку"
		case inList:
			t.Reason = "Атака уже доступна или выучена"
		default:
			t.CanLearn = true
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := TMTargetsResponse{
		ItemID:     item.ID,
		ItemName:   item.Name,
		EffectType: item.EffectType,
		AttackID:   item.AttackID,
		AttackName: attackName,
		Targets:    targets,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type UseTMRequest struct {
	ItemID int `json:"item_id"`
}

type UseTMResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	AttackID   int    `json:"attack_id,omitempty"`
	AttackName string `json:"attack_name,omitempty"`
}

// UseTMHandler — использует TM/HM на покемоне команды и добавляет атаку в
// список «Доступно к изучению». Предмет тратится сразу (TM и HM одноразовые).
func UseTMHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userPokemonID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "Invalid pokemon ID", http.StatusBadRequest)
		return
	}

	var req UseTMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.ItemID <= 0 {
		http.Error(w, "item_id is required", http.StatusBadRequest)
		return
	}

	item, err := db.GetItemByID(req.ItemID)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if item.AttackID <= 0 || (item.EffectType != "tm" && item.EffectType != "hm") {
		http.Error(w, "Это не TM/HM-предмет", http.StatusBadRequest)
		return
	}

	var ownerID, pokemonID, position int
	err = db.DB.QueryRow(
		"SELECT user_id, pokemon_id, COALESCE(position, 0) FROM user_pokemons WHERE id = ?",
		userPokemonID,
	).Scan(&ownerID, &pokemonID, &position)
	if err != nil || ownerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if position < 1 || position > 6 {
		http.Error(w, "TM/HM можно использовать только на покемонов в команде", http.StatusConflict)
		return
	}

	var qty int
	err = db.DB.QueryRow(
		"SELECT quantity FROM user_items WHERE user_id = ? AND item_id = ?",
		userID, req.ItemID,
	).Scan(&qty)
	if err == sql.ErrNoRows || qty <= 0 {
		http.Error(w, "У вас нет этого предмета", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var one int
	err = db.DB.QueryRow(`
		SELECT 1 FROM pokemon_attacks
		WHERE pokemon_id = ? AND attack_id = ? AND learn_method_id = 2
		LIMIT 1
	`, pokemonID, item.AttackID).Scan(&one)
	if err == sql.ErrNoRows {
		http.Error(w, "Этот покемон не может выучить эту атаку", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var attackName string
	var attackPP int
	err = db.DB.QueryRow(
		"SELECT name, pp FROM attacks WHERE id = ? AND COALESCE(disabled, 0) = 0",
		item.AttackID,
	).Scan(&attackName, &attackPP)
	if err == sql.ErrNoRows {
		http.Error(w, "Атака не найдена или недоступна", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var inList int
	err = db.DB.QueryRow(`
		SELECT COUNT(*) FROM user_pokemon_learnable_attacks
		WHERE user_pokemon_id = ? AND attack_id = ?
	`, userPokemonID, item.AttackID).Scan(&inList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if inList > 0 {
		http.Error(w, "Атака уже доступна или выучена", http.StatusConflict)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Повторная проверка количества под транзакцией (защита от гонок).
	if err = tx.QueryRow(
		"SELECT quantity FROM user_items WHERE user_id = ? AND item_id = ?",
		userID, req.ItemID,
	).Scan(&qty); err != nil || qty <= 0 {
		http.Error(w, "У вас нет этого предмета", http.StatusConflict)
		return
	}

	res, err := tx.Exec(`
		INSERT OR IGNORE INTO user_pokemon_learnable_attacks
			(user_pokemon_id, attack_id, level_available, is_learned, position,
			 current_pp, source, source_item_id)
		VALUES (?, ?, 0, 0, NULL, ?, ?, ?)
	`, userPokemonID, item.AttackID, attackPP, item.EffectType, req.ItemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "Атака уже доступна или выучена", http.StatusConflict)
		return
	}

	if qty-1 <= 0 {
		_, err = tx.Exec("DELETE FROM user_items WHERE user_id = ? AND item_id = ?", userID, req.ItemID)
	} else {
		_, err = tx.Exec("UPDATE user_items SET quantity = quantity - 1 WHERE user_id = ? AND item_id = ?", userID, req.ItemID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := UseTMResponse{
		Success:    true,
		Message:    "Атака «" + attackName + "» добавлена в список «Доступно к изучению»",
		AttackID:   item.AttackID,
		AttackName: attackName,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
