package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
)

type TeamPokemonMove struct {
	Name        string `json:"name"`
	EngName     string `json:"eng_name"`
	TypeID      int    `json:"type_id"`
	CategoryID  int    `json:"category_id"`
	PP          int    `json:"pp"`
	MaxPP       int    `json:"max_pp"`
	Power       int    `json:"power"`
	Accuracy    int    `json:"accuracy"`
	Priority    int    `json:"priority"`
	TargetID    int    `json:"target_id"`
	Description string `json:"description"`
	Contact     bool   `json:"contact"`
	Sound       bool   `json:"sound"`
	Disabled    bool   `json:"disabled"`
}

type TeamPokemon struct {
	ID               int               `json:"id"`
	PokemonID        int               `json:"pokemon_id"`
	Name             string            `json:"name"`
	Nickname         *string           `json:"nickname,omitempty"`
	Level            int               `json:"level"`
	Experience       int               `json:"experience"`
	CurrentHP        int               `json:"current_hp"`
	MaxHP            int               `json:"max_hp"`
	Attack           int               `json:"attack"`
	Defense          int               `json:"defense"`
	Speed            int               `json:"speed"`
	SpecialAttack    int               `json:"special_attack"`
	SpecialDefense   int               `json:"special_defense"`
	IVHP             int               `json:"iv_hp"`
	IVAttack         int               `json:"iv_attack"`
	IVDefense        int               `json:"iv_defense"`
	IVSpeed          int               `json:"iv_speed"`
	IVSpecialAttack  int               `json:"iv_special_attack"`
	IVSpecialDefense int               `json:"iv_special_defense"`
	EVHP             int               `json:"ev_hp"`
	EVAttack         int               `json:"ev_attack"`
	EVDefense        int               `json:"ev_defense"`
	EVSpeed          int               `json:"ev_speed"`
	EVSpecialAttack  int               `json:"ev_special_attack"`
	EVSpecialDefense int               `json:"ev_special_defense"`
	AvailableEV      int               `json:"available_ev"`
	Position         int               `json:"position"`
	StatusID         int               `json:"status_id"`
	NatureID         int               `json:"nature_id"`
	NatureName       string            `json:"nature_name"`
	IncreasedStat    string            `json:"increased_stat"`
	DecreasedStat    string            `json:"decreased_stat"`
	ImageURL         *string           `json:"image_url,omitempty"`
	PrimaryTypeID    int               `json:"primary_type_id"`
	SecondaryTypeID  *int              `json:"secondary_type_id,omitempty"`
	AbilityName      string            `json:"ability_name"`
	ExperienceGroup  int               `json:"experience_group"`
	Moves            []TeamPokemonMove `json:"moves"`
}

func GetUserTeam(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := db.DB.Query(`
        SELECT up.id, up.pokemon_id, p.name, up.nickname, up.level, up.experience,
               up.current_hp, up.max_hp, up.attack, up.defense, up.speed,
               up.special_attack, up.special_defense,
               up.iv_hp, up.iv_attack, up.iv_defense, up.iv_speed,
               up.iv_special_attack, up.iv_special_defense,
               up.ev_hp, up.ev_attack, up.ev_defense, up.ev_speed,
               up.ev_special_attack, up.ev_special_defense, up.available_ev,
               up.position, up.status_id, up.nature_id, n.name,
               COALESCE(n.increased_stat, ''),
               COALESCE(n.decreased_stat, ''),
                p.image_url,
                p.primary_type_id,
                p.secondary_type_id,
                p.experience_group,
                COALESCE((SELECT a.name FROM pokemon_abilities pa
                 JOIN abilities a ON pa.ability_id = a.id
                 WHERE pa.pokemon_id = p.id AND pa.is_hidden = 0 LIMIT 1), '') as ability_name
        FROM user_pokemons up
        JOIN pokemons p ON up.pokemon_id = p.id
        LEFT JOIN natures n ON up.nature_id = n.id
        WHERE up.user_id = ? AND up.position BETWEEN 1 AND 6
        ORDER BY up.position`, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var team []TeamPokemon
	for rows.Next() {
		var t TeamPokemon
		var nickname sql.NullString
		var imageURL sql.NullString
		var secondaryTypeID sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.PokemonID, &t.Name, &nickname, &t.Level, &t.Experience,
			&t.CurrentHP, &t.MaxHP, &t.Attack, &t.Defense, &t.Speed,
			&t.SpecialAttack, &t.SpecialDefense,
			&t.IVHP, &t.IVAttack, &t.IVDefense, &t.IVSpeed,
			&t.IVSpecialAttack, &t.IVSpecialDefense,
			&t.EVHP, &t.EVAttack, &t.EVDefense, &t.EVSpeed,
			&t.EVSpecialAttack, &t.EVSpecialDefense, &t.AvailableEV,
			&t.Position, &t.StatusID, &t.NatureID, &t.NatureName,
			&t.IncreasedStat, &t.DecreasedStat,
			&imageURL,
			&t.PrimaryTypeID, &secondaryTypeID, &t.ExperienceGroup, &t.AbilityName); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		if nickname.Valid {
			t.Nickname = &nickname.String
		}
		if imageURL.Valid {
			t.ImageURL = &imageURL.String
		}
		if secondaryTypeID.Valid {
			st := int(secondaryTypeID.Int64)
			t.SecondaryTypeID = &st
		}
		t.Moves = loadLearnedMoves(t.ID)
		team = append(team, t)
	}

	if team == nil {
		team = []TeamPokemon{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(team)
}

func loadLearnedMoves(userPokemonID int) []TeamPokemonMove {
	rows, err := db.DB.Query(`
		SELECT a.name, COALESCE(a.eng_name, ''), a.type_id, a.category_id,
		       COALESCE(ula.current_pp, a.pp), a.pp, COALESCE(a.power, 0), COALESCE(a.accuracy, 0),
		       COALESCE(a.priority, 0), COALESCE(a.target_id, 1),
		       COALESCE(a.description, ''),
		       COALESCE(a.contact, 0), COALESCE(a.sound, 0),
		       COALESCE(a.disabled, 0)
		FROM user_pokemon_learnable_attacks ula
		JOIN attacks a ON ula.attack_id = a.id
		WHERE ula.user_pokemon_id = ? AND ula.is_learned = 1 AND ula.position IS NOT NULL
		ORDER BY ula.position
	`, userPokemonID)
	if err != nil {
		return []TeamPokemonMove{}
	}
	defer rows.Close()
	moves := make([]TeamPokemonMove, 0, 4)
	for rows.Next() {
		var m TeamPokemonMove
		var disabled int
		if err := rows.Scan(&m.Name, &m.EngName, &m.TypeID, &m.CategoryID,
			&m.PP, &m.MaxPP, &m.Power, &m.Accuracy,
			&m.Priority, &m.TargetID, &m.Description,
			&m.Contact, &m.Sound, &disabled); err != nil {
			continue
		}
		m.Disabled = disabled != 0
		moves = append(moves, m)
	}
	return moves
}

