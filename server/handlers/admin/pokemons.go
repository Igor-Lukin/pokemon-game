package admin

import (
	"database/sql"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"strconv"
	"text/template"
)

type Pokemon struct {
	ID                 int
	Name               string
	Description        string
	ImageURL           sql.NullString
	Height             sql.NullFloat64
	Weight             sql.NullFloat64
	BaseHP             int
	BaseAttack         int
	BaseDefense        int
	BaseSpeed          int
	BaseSpecialAttack  int
	BaseSpecialDefense int
	CategoryPower      int
	ExperienceGroup    int
	Rarity             int
	FemalePercentage   int
	MalePercentage     int
	PrimaryTypeID      int
	SecondaryTypeID    sql.NullInt64
	EvolvesFromID      sql.NullInt64
	EvolutionMethodID  sql.NullInt64
	EvolutionLevel     sql.NullInt64
	EvolutionItemID    sql.NullInt64
}

// Список покемонов
func PokemonsHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	rows, err := db.DB.Query(`
        SELECT p.id, p.name, p.description, t1.name as primary_type, t2.name as secondary_type
        FROM pokemons p
        LEFT JOIN types t1 ON p.primary_type_id = t1.id
        LEFT JOIN types t2 ON p.secondary_type_id = t2.id
        ORDER BY p.id
    `)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type PokemonListItem struct {
		ID            int
		Name          string
		Description   string
		PrimaryType   string
		SecondaryType string
	}
	var list []PokemonListItem
	for rows.Next() {
		var item PokemonListItem
		var secondary sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.PrimaryType, &secondary); err != nil {
			continue
		}
		if secondary.Valid {
			item.SecondaryType = secondary.String
		} else {
			item.SecondaryType = "-"
		}
		list = append(list, item)
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/pokemons.html"))
	tmpl.Execute(w, list)
}

// Создание покемона (GET - форма, POST - сохранение)
func CreatePokemonHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		types := loadAttackTypes() // используем ту же функцию, что для атак
		data := struct {
			Pokemon Pokemon
			Types   []AttackType
		}{
			Pokemon: Pokemon{},
			Types:   types,
		}
		tmpl, err := template.ParseFiles("./server/templates/admin/pokemon_form.html")
		if err != nil {
			log.Printf("Template parse error: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		err = tmpl.Execute(w, data)
		if err != nil {
			log.Printf("Template execute error: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		description := r.FormValue("description")
		height := r.FormValue("height")
		weight := r.FormValue("weight")
		baseHP := r.FormValue("base_hp")
		baseAttack := r.FormValue("base_attack")
		baseDefense := r.FormValue("base_defense")
		baseSpeed := r.FormValue("base_speed")
		baseSpecialAttack := r.FormValue("base_special_attack")
		baseSpecialDefense := r.FormValue("base_special_defense")
		categoryPower := r.FormValue("category_power")
		experienceGroup := r.FormValue("experience_group")
		rarity := r.FormValue("rarity")
		femalePercentage := r.FormValue("female_percentage")
		malePercentage := r.FormValue("male_percentage")
		primaryTypeID := r.FormValue("primary_type_id")
		secondaryTypeID := r.FormValue("secondary_type_id")
		evolvesFromID := r.FormValue("evolves_from_id")
		evolutionMethodID := r.FormValue("evolution_method_id")
		evolutionLevel := r.FormValue("evolution_level")
		evolutionItemID := r.FormValue("evolution_item_id")
		imageURL := r.FormValue("image_url")

		if name == "" || primaryTypeID == "" {
			http.Error(w, "Name and Primary Type are required", http.StatusBadRequest)
			return
		}

		imageURLVal := nullString(imageURL)
		heightVal := nullFloat(height)
		weightVal := nullFloat(weight)
		baseHPVal, _ := strconv.Atoi(baseHP)
		baseAttackVal, _ := strconv.Atoi(baseAttack)
		baseDefenseVal, _ := strconv.Atoi(baseDefense)
		baseSpeedVal, _ := strconv.Atoi(baseSpeed)
		baseSpecialAttackVal, _ := strconv.Atoi(baseSpecialAttack)
		baseSpecialDefenseVal, _ := strconv.Atoi(baseSpecialDefense)
		categoryPowerVal, _ := strconv.Atoi(categoryPower)
		experienceGroupVal, _ := strconv.Atoi(experienceGroup)
		rarityVal, _ := strconv.Atoi(rarity)
		femalePercentageVal, _ := strconv.Atoi(femalePercentage)
		malePercentageVal, _ := strconv.Atoi(malePercentage)
		primaryTypeIDVal, _ := strconv.Atoi(primaryTypeID)
		secondaryTypeIDVal := nullInt(secondaryTypeID)
		evolvesFromIDVal := nullInt(evolvesFromID)
		evolutionMethodVal := nullInt(evolutionMethodID)
		evolutionLevelVal := nullInt(evolutionLevel)
		evolutionItemVal := nullInt(evolutionItemID)

		_, err := db.DB.Exec(`
			INSERT INTO pokemons (
				name, description, height, weight, base_hp, base_attack, base_defense, base_speed,
				base_special_attack, base_special_defense, category_power, experience_group, rarity,
				female_percentage, male_percentage, primary_type_id, secondary_type_id, evolves_from_id,
				evolution_method_id, evolution_level, evolution_item_id, image_url
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			name, description, heightVal, weightVal, baseHPVal, baseAttackVal, baseDefenseVal, baseSpeedVal,
			baseSpecialAttackVal, baseSpecialDefenseVal, categoryPowerVal, experienceGroupVal, rarityVal,
			femalePercentageVal, malePercentageVal, primaryTypeIDVal, secondaryTypeIDVal, evolvesFromIDVal,
			evolutionMethodVal, evolutionLevelVal, evolutionItemVal, imageURLVal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/pokemons", http.StatusSeeOther)
	}
}

// Редактирование покемона
func EditPokemonHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet {
		var p Pokemon
		err := db.DB.QueryRow(`
    SELECT id, name, description, height, weight, base_hp, base_attack, base_defense, base_speed,
           base_special_attack, base_special_defense, category_power, experience_group, rarity,
           female_percentage, male_percentage, primary_type_id, secondary_type_id,
           evolves_from_id, evolution_method_id, evolution_level, evolution_item_id, image_url
    FROM pokemons WHERE id = ?`, id).Scan(
			&p.ID, &p.Name, &p.Description, &p.Height, &p.Weight,
			&p.BaseHP, &p.BaseAttack, &p.BaseDefense, &p.BaseSpeed,
			&p.BaseSpecialAttack, &p.BaseSpecialDefense, &p.CategoryPower,
			&p.ExperienceGroup, &p.Rarity, &p.FemalePercentage, &p.MalePercentage,
			&p.PrimaryTypeID, &p.SecondaryTypeID, &p.EvolvesFromID, &p.EvolutionMethodID,
			&p.EvolutionLevel, &p.EvolutionItemID, &p.ImageURL)

		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Pokemon not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		types := loadAttackTypes()

		// внутри GET-блока после загрузки основных данных
		// Загружаем способности покемона
		abilityRows, err := db.DB.Query(`
		SELECT a.id, a.name, pa.is_hidden, COALESCE(pa.slot, 0)
		FROM pokemon_abilities pa
		JOIN abilities a ON pa.ability_id = a.id
		WHERE pa.pokemon_id = ?
		ORDER BY COALESCE(pa.slot, 99), a.name`, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer abilityRows.Close()
		type PokemonAbility struct {
			ID       int
			Name     string
			IsHidden bool
			Slot     int // 1=базовая X, 2=базовая Y, 3=скрытая, 0=не задан
		}
		var abilities []PokemonAbility
		for abilityRows.Next() {
			var ab PokemonAbility
			if err := abilityRows.Scan(&ab.ID, &ab.Name, &ab.IsHidden, &ab.Slot); err != nil {
				continue
			}
			abilities = append(abilities, ab)
		}
		// Загружаем все способности для выпадающего списка
		allAbilitiesRows, err := db.DB.Query("SELECT id, name FROM abilities ORDER BY name")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allAbilitiesRows.Close()
		type AllAbility struct {
			ID   int
			Name string
		}
		var allAbilities []AllAbility
		for allAbilitiesRows.Next() {
			var aa AllAbility
			if err := allAbilitiesRows.Scan(&aa.ID, &aa.Name); err == nil {
				allAbilities = append(allAbilities, aa)
			}
		}

		// Загружаем локации покемона
		locRows, err := db.DB.Query(`
			SELECT l.id, l.name, pl.rarity 
			FROM pokemon_locations pl
			JOIN locations l ON pl.location_id = l.id
			WHERE pl.pokemon_id = ?`, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer locRows.Close()
		type PokemonLocation struct {
			ID     int
			Name   string
			Rarity string
		}
		var locations []PokemonLocation
		for locRows.Next() {
			var loc PokemonLocation
			if err := locRows.Scan(&loc.ID, &loc.Name, &loc.Rarity); err != nil {
				continue
			}
			locations = append(locations, loc)
		}
		// Загружаем все локации для выпадающего списка
		allLocRows, err := db.DB.Query("SELECT id, name FROM locations ORDER BY name")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allLocRows.Close()
		type AllLocation struct {
			ID   int
			Name string
		}
		var allLocations []AllLocation
		for allLocRows.Next() {
			var al AllLocation
			if err := allLocRows.Scan(&al.ID, &al.Name); err == nil {
				allLocations = append(allLocations, al)
			}
		}

		// Загружаем атаки покемона
		attackRows, err := db.DB.Query(`
			SELECT a.id, a.name, lm.name, pa.level_learned_at, pa.learn_method_id
			FROM pokemon_attacks pa
			JOIN attacks a ON pa.attack_id = a.id
			JOIN attack_learn_methods lm ON pa.learn_method_id = lm.id
			WHERE pa.pokemon_id = ?`, id)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer attackRows.Close()
		type PokemonAttack struct {
			ID             int
			Name           string
			LearnMethod    string
			LearnMethodID  int
			LevelLearnedAt sql.NullInt64
		}
		var attacks []PokemonAttack
		for attackRows.Next() {
			var atk PokemonAttack
			if err := attackRows.Scan(&atk.ID, &atk.Name, &atk.LearnMethod, &atk.LevelLearnedAt, &atk.LearnMethodID); err != nil {
				continue
			}
			attacks = append(attacks, atk)
		}
		// Загружаем все атаки для выпадающего списка
		allAttacksRows, err := db.DB.Query("SELECT id, name FROM attacks ORDER BY name")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allAttacksRows.Close()
		type AllAttack struct {
			ID   int
			Name string
		}
		var allAttacks []AllAttack
		for allAttacksRows.Next() {
			var aa AllAttack
			if err := allAttacksRows.Scan(&aa.ID, &aa.Name); err == nil {
				allAttacks = append(allAttacks, aa)
			}
		}
		// Загружаем методы изучения
		methodsRows, err := db.DB.Query("SELECT id, name FROM attack_learn_methods ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer methodsRows.Close()
		type LearnMethod struct {
			ID   int
			Name string
		}
		var learnMethods []LearnMethod
		for methodsRows.Next() {
			var lm LearnMethod
			if err := methodsRows.Scan(&lm.ID, &lm.Name); err == nil {
				learnMethods = append(learnMethods, lm)
			}
		}

		// Загружаем всех покемонов для выбора предка (эволюция)
		allPokemonsRows, err := db.DB.Query("SELECT id, name FROM pokemons ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allPokemonsRows.Close()
		type SimplePokemon struct {
			ID   int
			Name string
		}
		var allPokemons []SimplePokemon
		for allPokemonsRows.Next() {
			var sp SimplePokemon
			if err := allPokemonsRows.Scan(&sp.ID, &sp.Name); err == nil {
				allPokemons = append(allPokemons, sp)
			}
		}
		// Загружаем методы эволюции
		evoMethodsRows, err := db.DB.Query("SELECT id, name FROM evolution_methods ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer evoMethodsRows.Close()
		type EvoMethod struct {
			ID   int
			Name string
		}
		var evoMethods []EvoMethod
		for evoMethodsRows.Next() {
			var em EvoMethod
			if err := evoMethodsRows.Scan(&em.ID, &em.Name); err == nil {
				evoMethods = append(evoMethods, em)
			}
		}

		// Добавьте эти поля в структуру data
		data := struct {
			Pokemon      Pokemon
			Types        []AttackType
			Abilities    []PokemonAbility
			AllAbilities []AllAbility
			Locations    []PokemonLocation
			AllLocations []AllLocation
			Attacks      []PokemonAttack
			AllAttacks   []AllAttack
			LearnMethods []LearnMethod
			AllPokemons  []SimplePokemon // новое
			EvoMethods   []EvoMethod     // новое
		}{p, types, abilities, allAbilities, locations, allLocations, attacks, allAttacks, learnMethods, allPokemons, evoMethods}

		tmpl := template.Must(template.ParseFiles("./server/templates/admin/pokemon_form.html"))
		tmpl.Execute(w, data)
		return
	}

	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		description := r.FormValue("description")
		height := r.FormValue("height")
		weight := r.FormValue("weight")
		baseHP := r.FormValue("base_hp")
		baseAttack := r.FormValue("base_attack")
		baseDefense := r.FormValue("base_defense")
		baseSpeed := r.FormValue("base_speed")
		baseSpecialAttack := r.FormValue("base_special_attack")
		baseSpecialDefense := r.FormValue("base_special_defense")
		categoryPower := r.FormValue("category_power")
		experienceGroup := r.FormValue("experience_group")
		rarity := r.FormValue("rarity")
		femalePercentage := r.FormValue("female_percentage")
		malePercentage := r.FormValue("male_percentage")
		primaryTypeID := r.FormValue("primary_type_id")
		secondaryTypeID := r.FormValue("secondary_type_id")
		evolvesFromID := r.FormValue("evolves_from_id")
		evolutionMethodID := r.FormValue("evolution_method_id")
		evolutionLevel := r.FormValue("evolution_level")
		evolutionItemID := r.FormValue("evolution_item_id")
		imageURL := r.FormValue("image_url")

		if name == "" || primaryTypeID == "" {
			http.Error(w, "Name and Primary Type are required", http.StatusBadRequest)
			return
		}

		imageURLVal := nullString(imageURL)
		heightVal := nullFloat(height)
		weightVal := nullFloat(weight)
		baseHPVal, _ := strconv.Atoi(baseHP)
		baseAttackVal, _ := strconv.Atoi(baseAttack)
		baseDefenseVal, _ := strconv.Atoi(baseDefense)
		baseSpeedVal, _ := strconv.Atoi(baseSpeed)
		baseSpecialAttackVal, _ := strconv.Atoi(baseSpecialAttack)
		baseSpecialDefenseVal, _ := strconv.Atoi(baseSpecialDefense)
		categoryPowerVal, _ := strconv.Atoi(categoryPower)
		experienceGroupVal, _ := strconv.Atoi(experienceGroup)
		rarityVal, _ := strconv.Atoi(rarity)
		femalePercentageVal, _ := strconv.Atoi(femalePercentage)
		malePercentageVal, _ := strconv.Atoi(malePercentage)
		primaryTypeIDVal, _ := strconv.Atoi(primaryTypeID)
		secondaryTypeIDVal := nullInt(secondaryTypeID)
		evolvesFromIDVal := nullInt(evolvesFromID)
		evolutionMethodVal := nullInt(evolutionMethodID)
		evolutionLevelVal := nullInt(evolutionLevel)
		evolutionItemVal := nullInt(evolutionItemID)

		_, err := db.DB.Exec(`
    UPDATE pokemons SET
        name=?, description=?, height=?, weight=?, base_hp=?, base_attack=?, base_defense=?, base_speed=?,
        base_special_attack=?, base_special_defense=?, category_power=?, experience_group=?, rarity=?,
        female_percentage=?, male_percentage=?, primary_type_id=?, secondary_type_id=?, evolves_from_id=?,
        evolution_method_id=?, evolution_level=?, evolution_item_id=?, image_url=?
    WHERE id=?`,
			name, description, heightVal, weightVal, baseHPVal, baseAttackVal, baseDefenseVal, baseSpeedVal,
			baseSpecialAttackVal, baseSpecialDefenseVal, categoryPowerVal, experienceGroupVal, rarityVal,
			femalePercentageVal, malePercentageVal, primaryTypeIDVal, secondaryTypeIDVal, evolvesFromIDVal,
			evolutionMethodVal, evolutionLevelVal, evolutionItemVal, imageURLVal, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/pokemons", http.StatusSeeOther)
	}
}

// Удаление покемона
func DeletePokemonHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.FormValue("id")
	if idStr == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	_, err = db.DB.Exec("DELETE FROM pokemons WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pokemons", http.StatusSeeOther)
}

// Добавление способности покемону
func AddPokemonAbilityHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Парсим форму (на случай, если данные не распарсились автоматически)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Parse error", http.StatusBadRequest)
		return
	}
	pokemonID := r.FormValue("pokemon_id")
	abilityID := r.FormValue("ability_id")
	// Слот ведёт согласованность: 1=базовая X, 2=базовая Y, 3=скрытая. is_hidden
	// выводится из слота (slot=3 <=> is_hidden=1) — держим синхронно (§7 ТЗ).
	slot, _ := strconv.Atoi(r.FormValue("slot"))
	if slot < 1 || slot > 3 {
		slot = 1
	}
	isHidden := slot == 3
	// Логируем
	log.Printf("AddAbility: pokemonID=%s, abilityID=%s, slot=%d, isHidden=%v", pokemonID, abilityID, slot, isHidden)
	if pokemonID == "" || abilityID == "" {
		log.Println("Missing parameters")
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	pID, err := strconv.Atoi(pokemonID)
	if err != nil {
		log.Printf("Invalid pokemonID: %v", err)
		http.Error(w, "Invalid pokemon id", http.StatusBadRequest)
		return
	}
	aID, err := strconv.Atoi(abilityID)
	if err != nil {
		log.Printf("Invalid abilityID: %v", err)
		http.Error(w, "Invalid ability id", http.StatusBadRequest)
		return
	}
	// Проверка на дубликат
	var count int
	db.DB.QueryRow("SELECT COUNT(*) FROM pokemon_abilities WHERE pokemon_id = ? AND ability_id = ?", pID, aID).Scan(&count)
	if count > 0 {
		log.Printf("Duplicate ability for pokemon %d, ability %d", pID, aID)
		http.Error(w, "Ability already assigned", http.StatusConflict)
		return
	}
	_, err = db.DB.Exec("INSERT INTO pokemon_abilities (pokemon_id, ability_id, is_hidden, slot) VALUES (?, ?, ?, ?)", pID, aID, isHidden, slot)
	if err != nil {
		log.Printf("DB error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Успешное добавление – редирект обратно на страницу редактирования
	redirectURL := "/admin/pokemons/edit?id=" + strconv.Itoa(pID)
	log.Printf("Redirecting to %s", redirectURL)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// Удаление способности у покемона
func RemovePokemonAbilityHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pokemonID := r.FormValue("pokemon_id")
	abilityID := r.FormValue("ability_id")
	if pokemonID == "" || abilityID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	pID, _ := strconv.Atoi(pokemonID)
	aID, _ := strconv.Atoi(abilityID)
	_, err := db.DB.Exec("DELETE FROM pokemon_abilities WHERE pokemon_id = ? AND ability_id = ?", pID, aID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pokemons/edit?id="+pokemonID, http.StatusSeeOther)
}

// Добавление локации покемону
func AddPokemonLocationHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pokemonID := r.FormValue("pokemon_id")
	locationID := r.FormValue("location_id")
	rarity := r.FormValue("rarity")
	if pokemonID == "" || locationID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	pID, _ := strconv.Atoi(pokemonID)
	lID, _ := strconv.Atoi(locationID)
	_, err := db.DB.Exec("INSERT INTO pokemon_locations (pokemon_id, location_id, rarity) VALUES (?, ?, ?)", pID, lID, rarity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pokemons/edit?id="+pokemonID, http.StatusSeeOther)
}

// Удаление локации у покемона
func RemovePokemonLocationHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pokemonID := r.FormValue("pokemon_id")
	locationID := r.FormValue("location_id")
	if pokemonID == "" || locationID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	pID, _ := strconv.Atoi(pokemonID)
	lID, _ := strconv.Atoi(locationID)
	_, err := db.DB.Exec("DELETE FROM pokemon_locations WHERE pokemon_id = ? AND location_id = ?", pID, lID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pokemons/edit?id="+pokemonID, http.StatusSeeOther)
}

// Добавление атаки покемону
func AddPokemonAttackHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pokemonID := r.FormValue("pokemon_id")
	attackID := r.FormValue("attack_id")
	learnMethodID := r.FormValue("learn_method_id")
	levelLearnedAt := r.FormValue("level_learned_at")
	if pokemonID == "" || attackID == "" || learnMethodID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	pID, _ := strconv.Atoi(pokemonID)
	aID, _ := strconv.Atoi(attackID)
	lmID, _ := strconv.Atoi(learnMethodID)
	var level interface{}
	if levelLearnedAt != "" {
		lvl, err := strconv.Atoi(levelLearnedAt)
		if err == nil {
			level = lvl
		} else {
			level = nil
		}
	} else {
		level = nil
	}
	// Проверка на дубликат
	var count int
	db.DB.QueryRow("SELECT COUNT(*) FROM pokemon_attacks WHERE pokemon_id = ? AND attack_id = ? AND learn_method_id = ?", pID, aID, lmID).Scan(&count)
	if count > 0 {
		http.Error(w, "Attack already assigned with this method", http.StatusConflict)
		return
	}
	_, err := db.DB.Exec("INSERT INTO pokemon_attacks (pokemon_id, attack_id, learn_method_id, level_learned_at) VALUES (?, ?, ?, ?)", pID, aID, lmID, level)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pokemons/edit?id="+pokemonID, http.StatusSeeOther)
}

// Удаление атаки у покемона
func RemovePokemonAttackHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pokemonID := r.FormValue("pokemon_id")
	attackID := r.FormValue("attack_id")
	learnMethodID := r.FormValue("learn_method_id")
	if pokemonID == "" || attackID == "" || learnMethodID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	pID, _ := strconv.Atoi(pokemonID)
	aID, _ := strconv.Atoi(attackID)
	lmID, _ := strconv.Atoi(learnMethodID)
	_, err := db.DB.Exec("DELETE FROM pokemon_attacks WHERE pokemon_id = ? AND attack_id = ? AND learn_method_id = ?", pID, aID, lmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pokemons/edit?id="+pokemonID, http.StatusSeeOther)
}

// Вспомогательные функции для преобразования в sql.NullFloat64 и sql.NullInt64
func nullFloat(s string) sql.NullFloat64 {
	if s == "" {
		return sql.NullFloat64{Valid: false}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return sql.NullFloat64{Valid: false}
	}
	return sql.NullFloat64{Float64: v, Valid: true}
}

func nullInt(s string) sql.NullInt64 {
	if s == "" {
		return sql.NullInt64{Valid: false}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
