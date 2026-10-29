package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"
)

type NurseryPokemon struct {
	ID        int    `json:"id"`
	PokemonID int    `json:"pokemon_id"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
	Gender    string `json:"gender"`
	ImageURL  string `json:"image_url"`
}

func GetUserPokemonsForNursery(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	search := r.URL.Query().Get("q")

	var rows *sql.Rows
	var err error
	if search != "" {
		id, convErr := strconv.Atoi(search)
		if convErr == nil {
			rows, err = db.DB.Query(`
				SELECT up.id, up.pokemon_id, p.name, up.level, COALESCE(up.gender, ''), p.image_url
				FROM user_pokemons up
				JOIN pokemons p ON up.pokemon_id = p.id
				WHERE up.user_id = ? AND (up.position = 0 OR up.position IS NULL) AND (up.pokemon_id = ? OR p.name LIKE ?)
				ORDER BY up.created_at`, userID, id, "%"+search+"%")
		} else {
			rows, err = db.DB.Query(`
				SELECT up.id, up.pokemon_id, p.name, up.level, COALESCE(up.gender, ''), p.image_url
				FROM user_pokemons up
				JOIN pokemons p ON up.pokemon_id = p.id
				WHERE up.user_id = ? AND (up.position = 0 OR up.position IS NULL) AND p.name LIKE ?
				ORDER BY up.created_at`, userID, "%"+search+"%")
		}
	} else {
		rows, err = db.DB.Query(`
			SELECT up.id, up.pokemon_id, p.name, up.level, COALESCE(up.gender, ''), p.image_url
			FROM user_pokemons up
			JOIN pokemons p ON up.pokemon_id = p.id
			WHERE up.user_id = ? AND (up.position = 0 OR up.position IS NULL)
			ORDER BY up.created_at`, userID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := make([]NurseryPokemon, 0)
	for rows.Next() {
		var np NurseryPokemon
		var imageURL sql.NullString
		if err := rows.Scan(&np.ID, &np.PokemonID, &np.Name, &np.Level, &np.Gender, &imageURL); err != nil {
			continue
		}
		if imageURL.Valid {
			np.ImageURL = imageURL.String
		}
		list = append(list, np)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

type TakeToTeamRequest struct {
	UserPokemonID int `json:"user_pokemon_id"`
}

func TakeToTeamHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req TakeToTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var count int
	err := db.DB.QueryRow("SELECT COUNT(*) FROM user_pokemons WHERE user_id = ? AND position BETWEEN 1 AND 6", userID).Scan(&count)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if count >= 6 {
		http.Error(w, "В команде уже 6 покемонов", http.StatusConflict)
		return
	}

	var targetPosition int
	err = db.DB.QueryRow("SELECT position FROM user_pokemons WHERE id = ? AND user_id = ?", req.UserPokemonID, userID).Scan(&targetPosition)
	if err != nil {
		http.Error(w, "Pokemon not found", http.StatusNotFound)
		return
	}
	if targetPosition >= 1 && targetPosition <= 6 {
		http.Error(w, "Покемон уже в команде", http.StatusConflict)
		return
	}

	nextPos := count + 1
	_, err = db.DB.Exec("UPDATE user_pokemons SET position = ? WHERE id = ? AND user_id = ?", nextPos, req.UserPokemonID, userID)
	if err != nil {
		http.Error(w, "Failed to update", http.StatusInternalServerError)
		return
	}

	log.Printf("User %d took pokemon %d into team slot %d", userID, req.UserPokemonID, nextPos)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

type SendToNurseryRequest struct {
	UserPokemonID int `json:"user_pokemon_id"`
}

func SendToNurseryHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req SendToNurseryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	err := db.DB.QueryRow("SELECT id FROM user_pokemons WHERE id = ? AND user_id = ? AND position BETWEEN 1 AND 6", req.UserPokemonID, userID).Scan(new(int))
	if err != nil {
		http.Error(w, "Pokemon not found or not in team", http.StatusNotFound)
		return
	}

	_, err = db.DB.Exec("UPDATE user_pokemons SET position = 0, is_starter = 0 WHERE id = ? AND user_id = ?", req.UserPokemonID, userID)
	if err != nil {
		http.Error(w, "Failed to update", http.StatusInternalServerError)
		return
	}

	log.Printf("User %d sent pokemon %d to nursery", userID, req.UserPokemonID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
