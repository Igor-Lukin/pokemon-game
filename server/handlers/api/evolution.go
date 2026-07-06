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

type EvolutionInfo struct {
	CanEvolve      bool   `json:"can_evolve"`
	EvolvesToID    int    `json:"evolves_to_id,omitempty"`
	EvolvesToName  string `json:"evolves_to_name,omitempty"`
	EvolutionLevel int    `json:"evolution_level,omitempty"`
}

type EvolveResponse struct {
	Success      bool   `json:"success"`
	NewPokemonID int    `json:"new_pokemon_id,omitempty"`
	NewName      string `json:"new_name,omitempty"`
	NewImageURL  string `json:"new_image_url,omitempty"`
	Message      string `json:"message,omitempty"`
}

func checkEvolution(userPokemonID int) (*EvolutionInfo, error) {
	var pokemonID, level int
	err := db.DB.QueryRow("SELECT pokemon_id, level FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&pokemonID, &level)
	if err != nil {
		log.Printf("checkEvolution: query user_pokemons error for id=%d: %v", userPokemonID, err)
		return nil, err
	}
	log.Printf("checkEvolution: user_pokemon=%d pokemon_id=%d level=%d", userPokemonID, pokemonID, level)

	var evolvesToID, evolutionLevel int
	var evolvesToName string
	err = db.DB.QueryRow(`
		SELECT id, evolution_level, name FROM pokemons
		WHERE evolves_from_id = ? AND evolution_method_id = 1 AND evolution_level <= ?
		ORDER BY evolution_level
		LIMIT 1
	`, pokemonID, level).Scan(&evolvesToID, &evolutionLevel, &evolvesToName)
	if err != nil {
		log.Printf("checkEvolution: no evolution found for pokemon_id=%d level=%d: %v", pokemonID, level, err)
		return &EvolutionInfo{CanEvolve: false}, nil
	}
	log.Printf("checkEvolution: found evolution -> id=%d name=%s level=%d", evolvesToID, evolvesToName, evolutionLevel)

	return &EvolutionInfo{
		CanEvolve:      true,
		EvolvesToID:    evolvesToID,
		EvolvesToName:  evolvesToName,
		EvolutionLevel: evolutionLevel,
	}, nil
}

func GetEvolutionHandler(w http.ResponseWriter, r *http.Request) {
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

	info, err := checkEvolution(userPokemonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func EvolveHandler(w http.ResponseWriter, r *http.Request) {
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

	// Check evolution conditions
	info, err := checkEvolution(userPokemonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !info.CanEvolve {
		http.Error(w, "Эволюция не доступна", http.StatusConflict)
		return
	}

	newPokemonID := info.EvolvesToID

	// Get current stats, IVs, EVs, level, nature
	var level, natureID int
	var iv [6]int
	var ev [6]int
	err = db.DB.QueryRow(`
		SELECT level, nature_id,
		       iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
		       ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense
		FROM user_pokemons WHERE id = ?
	`, userPokemonID).Scan(&level, &natureID,
		&iv[0], &iv[1], &iv[2], &iv[3], &iv[4], &iv[5],
		&ev[0], &ev[1], &ev[2], &ev[3], &ev[4], &ev[5])
	if err != nil {
		http.Error(w, "Pokemon not found", http.StatusNotFound)
		return
	}

	// Get new base stats
	var base [6]int
	var newName, newImageURL string
	err = db.DB.QueryRow(`
		SELECT base_hp, base_attack, base_defense, base_speed, base_special_attack, base_special_defense,
		       name, image_url
		FROM pokemons WHERE id = ?
	`, newPokemonID).Scan(&base[0], &base[1], &base[2], &base[3], &base[4], &base[5],
		&newName, &newImageURL)
	if err != nil {
		http.Error(w, "Evolution target not found", http.StatusNotFound)
		return
	}

	natureFactors, err := utils.GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1, 1, 1, 1, 1, 1}
	}
	train := [6]float64{1, 1, 1, 1, 1, 1}

	_, maxHP, atk, def, spd, spAtk, spDef :=
		utils.CalculateStats(base, iv, ev, level, natureFactors, train)

	// Update user_pokemon
	_, err = db.DB.Exec(`
		UPDATE user_pokemons SET
			pokemon_id = ?,
			max_hp = ?, attack = ?, defense = ?, speed = ?,
			special_attack = ?, special_defense = ?,
			current_hp = max_hp
		WHERE id = ?
	`, newPokemonID, maxHP, atk, def, spd, spAtk, spDef, userPokemonID)
	if err != nil {
		log.Printf("EvolveHandler: update error: %v", err)
		http.Error(w, "Failed to evolve", http.StatusInternalServerError)
		return
	}

	// Re-init learnable attacks for the new form
	if err := db.InitLearnableAttacks(userPokemonID, newPokemonID, level); err != nil {
		log.Printf("EvolveHandler: InitLearnableAttacks error: %v", err)
	}

	log.Printf("User %d evolved pokemon %d to %s (pokemon_id %d)", userID, userPokemonID, newName, newPokemonID)

	resp := EvolveResponse{
		Success:      true,
		NewPokemonID: newPokemonID,
		NewName:      newName,
		NewImageURL:  newImageURL,
		Message:      "Поздравляем! Ваш покемон эволюционировал в " + newName + "!",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
