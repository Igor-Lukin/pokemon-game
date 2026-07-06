package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"strconv"

	"github.com/gorilla/mux"
)

// ---------- Структуры ----------
type PokemonFullInfo struct {
	ID              int             `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Height          *float64        `json:"height,omitempty"`
	Weight          *float64        `json:"weight,omitempty"`
	PrimaryType     string          `json:"primary_type"`
	SecondaryType   string          `json:"secondary_type,omitempty"`
	PrimaryTypeID   int             `json:"primary_type_id"`
	SecondaryTypeID *int            `json:"secondary_type_id,omitempty"`
	ImageURL        string          `json:"image_url"`
	CategoryPower   int             `json:"category_power"`
	ExperienceGroup int             `json:"experience_group"`
	BaseStats       map[string]int  `json:"base_stats"`
	EvolutionChain  []EvolutionLink `json:"evolution_chain"`
	Attacks         []AttackInfo    `json:"attacks"`
	Abilities       []AbilityInfo   `json:"abilities"`
	Locations       []LocationInfo  `json:"locations"`
}

type EvolutionLink struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

type AttackInfo struct {
	Name           string `json:"name"`
	EngName        string `json:"eng_name,omitempty"`
	LearnMethod    string `json:"learn_method"`
	Level          *int   `json:"level,omitempty"`
	CategoryID     int    `json:"category_id"`
	TypeID         int    `json:"type_id"`
	TypeName       string `json:"type_name"`
	Power          int    `json:"power,omitempty"`
	Accuracy       int    `json:"accuracy,omitempty"`
	PP             int    `json:"pp,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	TargetID       int    `json:"target_id,omitempty"`
	Description    string `json:"description,omitempty"`
	Contact        bool   `json:"contact"`
	Sound          bool   `json:"sound"`
	EffectID *int   `json:"effect_id,omitempty"`
	EffectChance   int    `json:"effect_chance"`
	Disabled       bool   `json:"disabled,omitempty"`
}

type AbilityInfo struct {
	Name     string `json:"name"`
	IsHidden bool   `json:"is_hidden"`
}

type LocationInfo struct {
	Name   string `json:"name"`
	Rarity string `json:"rarity"`
}

// ---------- Основной обработчик полной информации ----------
func GetPokemonFullInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	info, err := fetchPokemonFullInfo(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// ---------- Вспомогательные функции ----------
func fetchPokemonFullInfo(id int) (*PokemonFullInfo, error) {
	var info PokemonFullInfo
	var height, weight sql.NullFloat64
	var primaryType, secondaryType sql.NullString
	var imageURL sql.NullString
	var baseHP, baseAttack, baseDefense, baseSpeed, baseSpecialAttack, baseSpecialDefense int
	var secondaryTypeID sql.NullInt64

	query := `
        SELECT p.id, p.name, p.description, p.height, p.weight,
               t1.name, t2.name, p.image_url,
               p.category_power, p.experience_group,
               p.base_hp, p.base_attack, p.base_defense, p.base_speed,
               p.base_special_attack, p.base_special_defense,
               p.primary_type_id, p.secondary_type_id
        FROM pokemons p
        LEFT JOIN types t1 ON p.primary_type_id = t1.id
        LEFT JOIN types t2 ON p.secondary_type_id = t2.id
        WHERE p.id = ?`
	err := db.DB.QueryRow(query, id).Scan(
		&info.ID, &info.Name, &info.Description, &height, &weight,
		&primaryType, &secondaryType, &imageURL,
		&info.CategoryPower, &info.ExperienceGroup,
		&baseHP, &baseAttack, &baseDefense, &baseSpeed,
		&baseSpecialAttack, &baseSpecialDefense,
		&info.PrimaryTypeID, &secondaryTypeID)
	if err != nil {
		return nil, err
	}

	// Обработка NULL
	if height.Valid {
		info.Height = &height.Float64
	}
	if weight.Valid {
		info.Weight = &weight.Float64
	}
	info.PrimaryType = primaryType.String
	if secondaryType.Valid {
		info.SecondaryType = secondaryType.String
	}
	if imageURL.Valid {
		info.ImageURL = imageURL.String
	}
	if secondaryTypeID.Valid {
		val := int(secondaryTypeID.Int64)
		info.SecondaryTypeID = &val
	}

	info.BaseStats = map[string]int{
		"hp": baseHP, "attack": baseAttack, "defense": baseDefense,
		"speed": baseSpeed, "special_attack": baseSpecialAttack, "special_defense": baseSpecialDefense,
	}

	info.EvolutionChain = getEvolutionChain(id)
	info.Attacks = getPokemonAttacks(id)
	info.Abilities = getPokemonAbilities(id)
	info.Locations = getPokemonLocations(id)

	return &info, nil
}

// Эволюционная цепочка
func getEvolutionChain(pokemonID int) []EvolutionLink {
	// Находим корень
	root := pokemonID
	for {
		var prev sql.NullInt64
		err := db.DB.QueryRow("SELECT evolves_from_id FROM pokemons WHERE id = ?", root).Scan(&prev)
		if err != nil || !prev.Valid {
			break
		}
		root = int(prev.Int64)
	}
	// Строим цепочку
	chain := []EvolutionLink{}
	current := root
	for {
		var name, image sql.NullString
		err := db.DB.QueryRow("SELECT name, image_url FROM pokemons WHERE id = ?", current).Scan(&name, &image)
		if err != nil {
			break
		}
		link := EvolutionLink{ID: current, Name: name.String}
		if image.Valid {
			link.ImageURL = image.String
		}
		chain = append(chain, link)
		// Следующая эволюция
		var next int
		err = db.DB.QueryRow("SELECT id FROM pokemons WHERE evolves_from_id = ?", current).Scan(&next)
		if err != nil {
			break
		}
		current = next
	}
	return chain
}

// Атаки покемона
func getPokemonAttacks(pokemonID int) []AttackInfo {
	rows, err := db.DB.Query(`
        SELECT a.name, COALESCE(a.eng_name, ''), lm.name,
               CASE WHEN lm.name = 'tm' THEN a.tm_number ELSE pa.level_learned_at END,
               a.category_id, a.type_id, t.name, a.power,
               COALESCE(a.accuracy, 0), a.pp,
               COALESCE(a.priority, 0), COALESCE(a.target_id, 1),
               COALESCE(a.description, ''),
               a.contact, a.sound, a.effect_status_id, a.effect_chance,
               COALESCE(a.disabled, 0)
        FROM pokemon_attacks pa
        JOIN attacks a ON pa.attack_id = a.id
        JOIN attack_learn_methods lm ON pa.learn_method_id = lm.id
        JOIN types t ON a.type_id = t.id
        WHERE pa.pokemon_id = ?
        ORDER BY lm.id, pa.level_learned_at`, pokemonID)
	if err != nil {
		return []AttackInfo{}
	}
	defer rows.Close()

	attacks := make([]AttackInfo, 0)
	for rows.Next() {
		var atk AttackInfo
		var level sql.NullInt64
		var method string
		var effectStatus sql.NullInt64
		var engName string
		var disabled int
		err := rows.Scan(&atk.Name, &engName, &method, &level,
			&atk.CategoryID, &atk.TypeID, &atk.TypeName, &atk.Power,
			&atk.Accuracy, &atk.PP,
			&atk.Priority, &atk.TargetID, &atk.Description,
			&atk.Contact, &atk.Sound, &effectStatus, &atk.EffectChance,
			&disabled)
		if err != nil {
			continue
		}
		atk.EngName = engName
		atk.LearnMethod = method
		atk.Disabled = disabled != 0
		if level.Valid {
			lvl := int(level.Int64)
			atk.Level = &lvl
		}
		if effectStatus.Valid {
			val := int(effectStatus.Int64)
			atk.EffectID = &val
		}
		attacks = append(attacks, atk)
	}
	return attacks
}

// Способности покемона
func getPokemonAbilities(pokemonID int) []AbilityInfo {
	rows, err := db.DB.Query(`
        SELECT a.name, pa.is_hidden
        FROM pokemon_abilities pa
        JOIN abilities a ON pa.ability_id = a.id
        WHERE pa.pokemon_id = ?
        ORDER BY pa.is_hidden`, pokemonID)
	if err != nil {
		return []AbilityInfo{}
	}
	defer rows.Close()
	abilities := make([]AbilityInfo, 0)
	for rows.Next() {
		var ab AbilityInfo
		rows.Scan(&ab.Name, &ab.IsHidden)
		abilities = append(abilities, ab)
	}
	return abilities
}

// Локации обитания
func getPokemonLocations(pokemonID int) []LocationInfo {
	rows, err := db.DB.Query(`
        SELECT l.name, pl.rarity
        FROM pokemon_locations pl
        JOIN locations l ON pl.location_id = l.id
        WHERE pl.pokemon_id = ?`, pokemonID)
	if err != nil {
		return []LocationInfo{}
	}
	defer rows.Close()
	locs := make([]LocationInfo, 0)
	for rows.Next() {
		var loc LocationInfo
		rows.Scan(&loc.Name, &loc.Rarity)
		locs = append(locs, loc)
	}
	return locs
}

// ---------- Остальные API (поиск, простой покемон, список) ----------
type PokemonInfo struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Height        *float64 `json:"height,omitempty"`
	Weight        *float64 `json:"weight,omitempty"`
	PrimaryType   string   `json:"primary_type"`
	SecondaryType string   `json:"secondary_type,omitempty"`
	ImageURL      string   `json:"image_url"`
}

func GetPokemonInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	var info PokemonInfo
	var height, weight sql.NullFloat64
	var primaryType, secondaryType sql.NullString
	var imageURL sql.NullString
	err = db.DB.QueryRow(`
        SELECT p.id, p.name, p.description, p.height, p.weight,
               t1.name, t2.name, p.image_url
        FROM pokemons p
        LEFT JOIN types t1 ON p.primary_type_id = t1.id
        LEFT JOIN types t2 ON p.secondary_type_id = t2.id
        WHERE p.id = ?`, id).Scan(
		&info.ID, &info.Name, &info.Description, &height, &weight,
		&primaryType, &secondaryType, &imageURL)
	if err != nil {
		http.Error(w, "Pokemon not found", http.StatusNotFound)
		return
	}
	if height.Valid {
		info.Height = &height.Float64
	}
	if weight.Valid {
		info.Weight = &weight.Float64
	}
	info.PrimaryType = primaryType.String
	if secondaryType.Valid {
		info.SecondaryType = secondaryType.String
	}
	if imageURL.Valid {
		info.ImageURL = imageURL.String
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

type PokemonSearchResult struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	PrimaryTypeID   int    `json:"primary_type_id"`
	SecondaryTypeID *int   `json:"secondary_type_id,omitempty"`
	ImageURL        string `json:"image_url"`
}

func SearchPokemons(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing query", http.StatusBadRequest)
		return
	}
	sqlQuery := `
        SELECT id, name, description, primary_type_id, secondary_type_id, COALESCE(image_url, '')
        FROM pokemons
        WHERE id = ? OR LOWER(name) LIKE LOWER(?)
        LIMIT 20`
	likePattern := "%" + query + "%"
	rows, err := db.DB.Query(sqlQuery, query, likePattern)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var results []PokemonSearchResult
	for rows.Next() {
		var r PokemonSearchResult
		var secondary sql.NullInt64
		var imageURL string
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.PrimaryTypeID, &secondary, &imageURL); err != nil {
			continue
		}
		r.ImageURL = imageURL
		if secondary.Valid {
			val := int(secondary.Int64)
			r.SecondaryTypeID = &val
		}
		results = append(results, r)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func GetAllPokemons(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query("SELECT id, name FROM pokemons ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type SimplePokemon struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	var list []SimplePokemon
	for rows.Next() {
		var sp SimplePokemon
		rows.Scan(&sp.ID, &sp.Name)
		list = append(list, sp)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}
