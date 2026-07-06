package api

import (
	"encoding/json"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"pokemon-game/server/utils"
	"strconv"

	"github.com/gorilla/mux"
)

type EVRequest struct {
	Stat   string `json:"stat"`
	Amount int    `json:"amount"`
}

type EVResponse struct {
	Stat    string `json:"stat"`
	NewEV   int    `json:"new_ev"`
	NewStat int    `json:"new_stat"`
	AvailableEV int `json:"available_ev"`
}

func DistributeEVHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	pokemonIDStr := vars["id"]
	pokemonID, err := strconv.Atoi(pokemonIDStr)
	if err != nil {
		http.Error(w, "Invalid pokemon id", http.StatusBadRequest)
		return
	}

	// Verify ownership
	var ownerID int
	err = db.DB.QueryRow("SELECT user_id FROM user_pokemons WHERE id = ?", pokemonID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		http.Error(w, "Pokemon not found", http.StatusNotFound)
		return
	}

	var req EVRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Amount <= 0 {
		http.Error(w, "Amount must be positive", http.StatusBadRequest)
		return
	}

	// Map stat name to column name
	statMap := map[string]string{
		"hp":              "ev_hp",
		"attack":          "ev_attack",
		"defense":         "ev_defense",
		"speed":           "ev_speed",
		"special_attack":  "ev_special_attack",
		"special_defense": "ev_special_defense",
	}
	statCol, ok := statMap[req.Stat]
	if !ok {
		http.Error(w, "Invalid stat", http.StatusBadRequest)
		return
	}

	// Read current EV data
	var currentStatEV, availableEV int
	query := "SELECT " + statCol + ", COALESCE(available_ev, 0) FROM user_pokemons WHERE id = ?"
	err = db.DB.QueryRow(query, pokemonID).Scan(&currentStatEV, &availableEV)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// Validation
	if availableEV < req.Amount {
		http.Error(w, "Недостаточно свободных EV", http.StatusBadRequest)
		return
	}
	if currentStatEV+req.Amount > 126 {
		http.Error(w, "В этом стате не может быть больше 126 EV", http.StatusBadRequest)
		return
	}

	// Update EV
	newStatEV := currentStatEV + req.Amount
	newAvailableEV := availableEV - req.Amount
	updateQuery := "UPDATE user_pokemons SET " + statCol + " = ?, available_ev = ? WHERE id = ?"
	_, err = db.DB.Exec(updateQuery, newStatEV, newAvailableEV, pokemonID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// Recalculate the stat
	baseStatIdx := map[string]int{
		"hp": 0, "attack": 1, "defense": 2,
		"speed": 3, "special_attack": 4, "special_defense": 5,
	}
	idx := baseStatIdx[req.Stat]

	var pokemonIDinDB, natureID, level int
	var base [6]int
	var iv [6]int
	var ev [6]int
	err = db.DB.QueryRow(`
		SELECT up.pokemon_id, up.nature_id, up.level,
		       up.iv_hp, up.iv_attack, up.iv_defense, up.iv_speed,
		       up.iv_special_attack, up.iv_special_defense,
		       up.ev_hp, up.ev_attack, up.ev_defense, up.ev_speed,
		       up.ev_special_attack, up.ev_special_defense
		FROM user_pokemons up WHERE up.id = ?
	`, pokemonID).Scan(&pokemonIDinDB, &natureID, &level,
		&iv[0], &iv[1], &iv[2], &iv[3], &iv[4], &iv[5],
		&ev[0], &ev[1], &ev[2], &ev[3], &ev[4], &ev[5])
	if err != nil {
		log.Printf("DistributeEV: query error: %v", err)
		// Return partial response even if recalc fails
		resp := EVResponse{
			Stat:        req.Stat,
			NewEV:       newStatEV,
			NewStat:     0,
			AvailableEV: newAvailableEV,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	err = db.DB.QueryRow(`
		SELECT base_hp, base_attack, base_defense, base_speed,
		       base_special_attack, base_special_defense
		FROM pokemons WHERE id = ?
	`, pokemonIDinDB).Scan(&base[0], &base[1], &base[2], &base[3], &base[4], &base[5])
	if err != nil {
		log.Printf("DistributeEV: base stats query error: %v", err)
		resp := EVResponse{
			Stat:        req.Stat,
			NewEV:       newStatEV,
			NewStat:     0,
			AvailableEV: newAvailableEV,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	natureFactors, err := utils.GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1, 1, 1, 1, 1, 1}
	}
	train := [6]float64{1, 1, 1, 1, 1, 1}

	_, maxHP, atk, def, spd, spAtk, spDef :=
		utils.CalculateStats(base, iv, ev, level, natureFactors, train)

	statValues := [6]int{maxHP, atk, def, spd, spAtk, spDef}
	statColumns := []string{"current_hp", "attack", "defense", "speed", "special_attack", "special_defense"}
	if idx == 0 {
		// HP: also update current_hp to match new max_hp
		_, err = db.DB.Exec("UPDATE user_pokemons SET max_hp = ?, current_hp = max_hp WHERE id = ?", statValues[0], pokemonID)
	} else {
		_, err = db.DB.Exec("UPDATE user_pokemons SET "+statColumns[idx]+" = ? WHERE id = ?", statValues[idx], pokemonID)
	}
	if err != nil {
		log.Printf("DistributeEV: stat update error: %v", err)
	}

	resp := EVResponse{
		Stat:        req.Stat,
		NewEV:       newStatEV,
		NewStat:     statValues[idx],
		AvailableEV: newAvailableEV,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
