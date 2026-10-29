package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"strconv"
	"strings"

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
	// Выбранная форма (пусто = базовая) и список всех форм вида.
	FormKey  string            `json:"form_key,omitempty"`
	FormName string            `json:"form_name,omitempty"`
	Forms    []PokemonFormInfo `json:"forms,omitempty"`
}

// PokemonFormInfo — форма вида для переключателя в покедексе.
type PokemonFormInfo struct {
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	ImageURL        string         `json:"image_url"`
	PrimaryType     string         `json:"primary_type"`
	SecondaryType   string         `json:"secondary_type,omitempty"`
	PrimaryTypeID   int            `json:"primary_type_id"`
	SecondaryTypeID *int           `json:"secondary_type_id,omitempty"`
	BaseStats       map[string]int `json:"base_stats"`
	Description     string         `json:"description,omitempty"`
	IsBase          bool           `json:"is_base"`
}

type EvolutionLink struct {
	ID         int             `json:"id"`
	Name       string          `json:"name"`
	ImageURL   string          `json:"image_url"`
	MethodID   int             `json:"method_id,omitempty"`
	MethodName string          `json:"method_name,omitempty"`
	Level      *int            `json:"level,omitempty"`
	ItemID     *int            `json:"item_id,omitempty"`
	ItemName   string          `json:"item_name,omitempty"`
	Condition  string          `json:"condition,omitempty"`
	Children   []EvolutionLink `json:"children,omitempty"`
}

type AttackInfo struct {
	Name         string `json:"name"`
	EngName      string `json:"eng_name,omitempty"`
	LearnMethod  string `json:"learn_method"`
	Level        *int   `json:"level,omitempty"`
	TMKind       string `json:"tm_kind,omitempty"`
	CategoryID   int    `json:"category_id"`
	TypeID       int    `json:"type_id"`
	TypeName     string `json:"type_name"`
	Power        int    `json:"power,omitempty"`
	Accuracy     int    `json:"accuracy,omitempty"`
	PP           int    `json:"pp,omitempty"`
	Priority     int    `json:"priority,omitempty"`
	TargetID     int    `json:"target_id,omitempty"`
	Description  string `json:"description,omitempty"`
	Contact      bool   `json:"contact"`
	Sound        bool   `json:"sound"`
	EffectID     *int   `json:"effect_id,omitempty"`
	EffectChance int    `json:"effect_chance"`
	Disabled     bool   `json:"disabled,omitempty"`
}

type AbilityInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsHidden    bool   `json:"is_hidden"`
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

	info, err := fetchPokemonFullInfo(id, r.URL.Query().Get("form"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// ---------- Вспомогательные функции ----------
func fetchPokemonFullInfo(id int, formKey string) (*PokemonFullInfo, error) {
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
	applyPokemonForms(&info, formKey)

	info.EvolutionChain = getEvolutionChain(id)
	info.Attacks = getPokemonAttacks(id)
	info.Abilities = getPokemonAbilities(id)
	info.Locations = getPokemonLocations(id)

	return &info, nil
}

// applyPokemonForms добавляет в ответ список форм и, если запрошена конкретная,
// подменяет спрайт/типы/базовые статы/описание. Данные — из pokemon_forms.
func applyPokemonForms(info *PokemonFullInfo, requested string) {
	rows := db.FormsForPokemon(info.ID)
	if len(rows) == 0 {
		return
	}
	baseStats := cloneStats(info.BaseStats)
	forms := make([]PokemonFormInfo, 0, len(rows))
	hasBase := false
	var selected *PokemonFormInfo

	appendBase := func(name string) {
		forms = append(forms, PokemonFormInfo{
			Key: "", Name: name, ImageURL: info.ImageURL,
			PrimaryType: info.PrimaryType, SecondaryType: info.SecondaryType,
			PrimaryTypeID: info.PrimaryTypeID, SecondaryTypeID: info.SecondaryTypeID,
			BaseStats: cloneStats(baseStats), Description: info.Description, IsBase: true,
		})
	}

	for i := range rows {
		f := &rows[i]
		label := f.FormLabel
		if label == "" {
			label = f.FormName
		}
		if f.IsBaseForm {
			hasBase = true
			appendBase(label)
			info.FormName = label
			if requested == f.FormKey {
				selected = nil
			}
			continue
		}

		stats := scaleFormStats(baseStats, f)
		primaryID := info.PrimaryTypeID
		if f.PrimaryTypeID > 0 {
			primaryID = f.PrimaryTypeID
		}
		var secondaryID *int
		if f.SecondaryTypeID > 0 {
			v := f.SecondaryTypeID
			secondaryID = &v
		} else {
			secondaryID = info.SecondaryTypeID
		}
		primaryName := info.PrimaryType
		if primaryID != info.PrimaryTypeID {
			primaryName = typeNameByID(primaryID)
		}
		secondaryName := info.SecondaryType
		if secondaryID != nil && (info.SecondaryTypeID == nil || *secondaryID != *info.SecondaryTypeID) {
			secondaryName = typeNameByID(*secondaryID)
		}
		image := info.ImageURL
		if f.ImageURL != "" {
			image = f.ImageURL
		}
		desc := info.Description
		if f.FormDescription != "" {
			desc = f.FormDescription
		}
		fi := PokemonFormInfo{
			Key: f.FormKey, Name: label, ImageURL: image,
			PrimaryType: primaryName, SecondaryType: secondaryName,
			PrimaryTypeID: primaryID, SecondaryTypeID: secondaryID,
			BaseStats: stats, Description: desc, IsBase: false,
		}
		forms = append(forms, fi)
		if requested == f.FormKey {
			copy := fi
			selected = &copy
		}
	}

	if !hasBase {
		base := PokemonFormInfo{
			Key: "", Name: "Обычная форма", ImageURL: info.ImageURL,
			PrimaryType: info.PrimaryType, SecondaryType: info.SecondaryType,
			PrimaryTypeID: info.PrimaryTypeID, SecondaryTypeID: info.SecondaryTypeID,
			BaseStats: cloneStats(baseStats), Description: info.Description, IsBase: true,
		}
		forms = append([]PokemonFormInfo{base}, forms...)
	}
	info.Forms = forms

	if selected != nil {
		info.FormKey = selected.Key
		info.FormName = selected.Name
		info.ImageURL = selected.ImageURL
		info.PrimaryType = selected.PrimaryType
		info.SecondaryType = selected.SecondaryType
		info.PrimaryTypeID = selected.PrimaryTypeID
		info.SecondaryTypeID = selected.SecondaryTypeID
		info.BaseStats = selected.BaseStats
		info.Description = selected.Description
	}
}

func cloneStats(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// scaleFormStats применяет множители формы к базовым статам вида.
func scaleFormStats(base map[string]int, f *db.BattleForm) map[string]int {
	scale := func(v int, mult float64) int {
		if mult <= 0 {
			mult = 1
		}
		return int(float64(v)*mult + 0.5)
	}
	return map[string]int{
		"hp":              base["hp"],
		"attack":          scale(base["attack"], f.AttackMult),
		"defense":         scale(base["defense"], f.DefenseMult),
		"speed":           scale(base["speed"], f.SpeedMult),
		"special_attack":  scale(base["special_attack"], f.SpecialAttackMult),
		"special_defense": scale(base["special_defense"], f.SpecialDefenseMult),
	}
}

func typeNameByID(id int) string {
	if id <= 0 {
		return ""
	}
	var name string
	db.DB.QueryRow("SELECT name FROM types WHERE id = ?", id).Scan(&name)
	return name
}

// Эволюционная цепочка. Возвращает корень семейства; ветвления лежат в
// Children (например, Кирлия -> Гардевуар и Галлейд).
func getEvolutionChain(pokemonID int) []EvolutionLink {
	// Находим корень
	root := pokemonID
	seen := map[int]bool{}
	for {
		var prev sql.NullInt64
		err := db.DB.QueryRow("SELECT evolves_from_id FROM pokemons WHERE id = ?", root).Scan(&prev)
		if err != nil || !prev.Valid {
			break
		}
		if seen[int(prev.Int64)] {
			break
		}
		seen[int(prev.Int64)] = true
		root = int(prev.Int64)
	}
	rootNode := buildEvolutionNode(root, map[int]bool{})
	if rootNode == nil {
		return []EvolutionLink{}
	}
	return []EvolutionLink{*rootNode}
}

func buildEvolutionNode(id int, visited map[int]bool) *EvolutionLink {
	if visited[id] {
		return nil
	}
	visited[id] = true

	var name, image sql.NullString
	var methodID, level, itemID sql.NullInt64
	var methodName, itemName, condition string
	err := db.DB.QueryRow(`
		SELECT p.name, p.image_url, p.evolution_method_id, p.evolution_level,
		       p.evolution_item_id, COALESCE(p.evolution_condition, ''),
		       COALESCE(em.name, ''), COALESCE(i.name, '')
		FROM pokemons p
		LEFT JOIN evolution_methods em ON em.id = p.evolution_method_id
		LEFT JOIN items i ON i.id = p.evolution_item_id
		WHERE p.id = ?`, id).Scan(
		&name, &image, &methodID, &level, &itemID,
		&condition, &methodName, &itemName)
	if err != nil {
		return nil
	}

	link := &EvolutionLink{ID: id, Name: name.String}
	if image.Valid {
		link.ImageURL = image.String
	}
	if methodID.Valid {
		link.MethodID = int(methodID.Int64)
		link.MethodName = methodName
	} else if level.Valid {
		// Старые записи (gen1) хранят только уровень — считаем метод level.
		link.MethodID = 1
		link.MethodName = "level"
	}
	if level.Valid {
		lvl := int(level.Int64)
		link.Level = &lvl
	}
	if itemID.Valid {
		it := int(itemID.Int64)
		link.ItemID = &it
		link.ItemName = itemName
	}
	link.Condition = condition

	// Собираем id детей до рекурсии, чтобы не держать открытыми rows.
	childIDs := []int{}
	rows, err := db.DB.Query(`
		SELECT id FROM pokemons
		WHERE evolves_from_id = ?
		ORDER BY COALESCE(evolution_priority, 0) DESC, id ASC`, id)
	if err == nil {
		for rows.Next() {
			var childID int
			if rows.Scan(&childID) == nil {
				childIDs = append(childIDs, childID)
			}
		}
		rows.Close()
	}
	for _, childID := range childIDs {
		if child := buildEvolutionNode(childID, visited); child != nil {
			link.Children = append(link.Children, *child)
		}
	}
	return link
}

// Атаки покемона
func getPokemonAttacks(pokemonID int) []AttackInfo {
	rows, err := db.DB.Query(`
        SELECT a.name, COALESCE(a.eng_name, ''), lm.name,
               CASE WHEN lm.name = 'tm' THEN
                   COALESCE(
                       (SELECT CASE WHEN it.effect_type = 'hm' THEN it.id - 2000 ELSE it.id - 1000 END
                        FROM items it
                        WHERE it.attack_id = a.id AND it.effect_type IN ('tm', 'hm')
                        ORDER BY it.id LIMIT 1),
                       NULLIF(a.tm_number, 0)
                   )
               ELSE pa.level_learned_at END,
               CASE WHEN lm.name = 'tm' THEN
                   COALESCE(
                       (SELECT it.effect_type FROM items it
                        WHERE it.attack_id = a.id AND it.effect_type IN ('tm', 'hm')
                        ORDER BY it.id LIMIT 1),
                       'tm'
                   )
               ELSE '' END,
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
		var method, tmKind string
		var effectStatus sql.NullInt64
		var engName string
		var disabled int
		err := rows.Scan(&atk.Name, &engName, &method, &level, &tmKind,
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
		atk.TMKind = tmKind
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
        SELECT a.name, COALESCE(a.description, ''), pa.is_hidden
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
		rows.Scan(&ab.Name, &ab.Description, &ab.IsHidden)
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
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "Missing query", http.StatusBadRequest)
		return
	}
	exactID, _ := strconv.Atoi(query)
	sqlQuery := `
        SELECT id, name, description, primary_type_id, secondary_type_id, COALESCE(image_url, '')
        FROM pokemons
        WHERE id = ? OR LOWER(name) LIKE LOWER(?) OR CAST(id AS TEXT) LIKE ?
        ORDER BY CASE WHEN id = ? THEN 0 WHEN LOWER(name) = LOWER(?) THEN 1 ELSE 2 END, id
        LIMIT 200`
	likePattern := "%" + query + "%"
	idPrefix := query + "%"
	rows, err := db.DB.Query(sqlQuery, exactID, likePattern, idPrefix, exactID, query)
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

// GetPokemonsByType возвращает всех покемонов, у которых указанный тип является
// основным или вторичным. Используется плиткой типов в покедексе.
func GetPokemonsByType(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	typeID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid type id", http.StatusBadRequest)
		return
	}
	rows, err := db.DB.Query(`
        SELECT id, name, description, primary_type_id, secondary_type_id, COALESCE(image_url, '')
        FROM pokemons
        WHERE primary_type_id = ? OR secondary_type_id = ?
        ORDER BY id`, typeID, typeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	results := make([]PokemonSearchResult, 0)
	for rows.Next() {
		var p PokemonSearchResult
		var secondary sql.NullInt64
		var imageURL string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.PrimaryTypeID, &secondary, &imageURL); err != nil {
			continue
		}
		p.ImageURL = imageURL
		if secondary.Valid {
			val := int(secondary.Int64)
			p.SecondaryTypeID = &val
		}
		results = append(results, p)
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
