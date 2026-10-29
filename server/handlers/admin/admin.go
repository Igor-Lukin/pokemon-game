package admin

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"pokemon-game/server/utils"
	"strconv"
)

// проверка, является ли текущий пользователь админом
func checkAdmin(r *http.Request) bool {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		return false
	}
	var isAdmin int
	err := db.DB.QueryRow("SELECT is_admin FROM users WHERE id = ?", userID).Scan(&isAdmin)
	if err != nil {
		return false
	}
	return isAdmin == 1
}

type regionOption struct {
	ID   int
	Name string
}

type weatherOption struct {
	ID   int
	Name string
}

type locationExitOption struct {
	ID   int
	Name string
}

type locationFormData struct {
	ID           int
	Name         string
	Description  string
	HasNursery   bool
	HasHealing   bool
	RegionID     int
	Regions      []regionOption
	Exits        []locationExitOption
	AllLocations []locationExitOption
}

type regionFormData struct {
	ID               int
	Name             string
	CurrentWeatherID int
	Weathers         []weatherOption
}

func loadRegionOptions() ([]regionOption, error) {
	rows, err := db.DB.Query("SELECT id, name FROM regions ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var regions []regionOption
	for rows.Next() {
		var r regionOption
		if err := rows.Scan(&r.ID, &r.Name); err == nil {
			regions = append(regions, r)
		}
	}
	return regions, nil
}

func loadWeatherOptions() ([]weatherOption, error) {
	rows, err := db.DB.Query("SELECT id, name FROM weather ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var weathers []weatherOption
	for rows.Next() {
		var w weatherOption
		if err := rows.Scan(&w.ID, &w.Name); err == nil {
			weathers = append(weathers, w)
		}
	}
	return weathers, nil
}

func formInt(value string, fallback int) int {
	n, err := strconv.Atoi(value)
	if err != nil || n == 0 {
		return fallback
	}
	return n
}

// главная страница админки
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/index.html"))
	tmpl.Execute(w, nil)
}

// список локаций
func LocationsHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	rows, err := db.DB.Query(`
		SELECT l.id, l.name, l.description, l.has_nursery, l.has_healing,
		       COALESCE(l.region_id, 1), COALESCE(r.name, '')
		FROM locations l
		LEFT JOIN regions r ON r.id = l.region_id
		ORDER BY l.id
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Location struct {
		ID          int
		Name        string
		Description string
		HasNursery  bool
		HasHealing  bool
		RegionID    int
		RegionName  string
	}
	var locations []Location
	for rows.Next() {
		var loc Location
		var nursery, healing int
		if err := rows.Scan(&loc.ID, &loc.Name, &loc.Description, &nursery, &healing, &loc.RegionID, &loc.RegionName); err != nil {
			continue
		}
		loc.HasNursery = nursery == 1
		loc.HasHealing = healing == 1
		locations = append(locations, loc)
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/locations.html"))
	tmpl.Execute(w, locations)
}

// создание новой локации (GET - форма, POST - сохранение)
func CreateLocationHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		regions, err := loadRegionOptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/location_form.html"))
		tmpl.Execute(w, locationFormData{RegionID: 2, Regions: regions})
		return
	}
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		description := r.FormValue("description")
		if name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}
		hasNursery := 0
		if r.FormValue("has_nursery") == "on" {
			hasNursery = 1
		}
		hasHealing := 0
		if r.FormValue("has_healing") == "on" {
			hasHealing = 1
		}
		regionID := formInt(r.FormValue("region_id"), 2)
		_, err := db.DB.Exec(
			"INSERT INTO locations (name, description, has_nursery, has_healing, region_id) VALUES (?, ?, ?, ?, ?)",
			name, description, hasNursery, hasHealing, regionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations", http.StatusSeeOther)
	}
}

// редактирование локации
func EditLocationHandler(w http.ResponseWriter, r *http.Request) {
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
		var name, description string
		var hasNursery, hasHealing, regionID int
		err := db.DB.QueryRow(`
			SELECT name, description, has_nursery, has_healing, COALESCE(region_id, 1)
			FROM locations WHERE id = ?
		`, id).Scan(&name, &description, &hasNursery, &hasHealing, &regionID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Location not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		// Получаем список выходов из этой локации
		rows, err := db.DB.Query("SELECT to_location, (SELECT name FROM locations WHERE id = to_location) FROM location_exits WHERE from_location = ?", id)
		if err != nil {
			log.Printf("Error loading exits: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var exits []locationExitOption
		for rows.Next() {
			var e locationExitOption
			if err := rows.Scan(&e.ID, &e.Name); err == nil {
				exits = append(exits, e)
			}
		}

		// Получаем список всех локаций для выпадающего списка добавления выхода
		allRows, err := db.DB.Query(`
			SELECT l.id, l.name || COALESCE(' — ' || r.name, '')
			FROM locations l
			LEFT JOIN regions r ON r.id = l.region_id
			ORDER BY l.id
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allRows.Close()
		var allLocations []locationExitOption
		for allRows.Next() {
			var loc locationExitOption
			if err := allRows.Scan(&loc.ID, &loc.Name); err == nil {
				allLocations = append(allLocations, loc)
			}
		}

		regions, err := loadRegionOptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := locationFormData{
			ID:           id,
			Name:         name,
			Description:  description,
			HasNursery:   hasNursery == 1,
			HasHealing:   hasHealing == 1,
			RegionID:     regionID,
			Regions:      regions,
			Exits:        exits,
			AllLocations: allLocations,
		}

		tmpl, err := template.ParseFiles("./server/templates/admin/location_form.html")
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
	}

	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		description := r.FormValue("description")
		if name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}
		hasNursery := 0
		if r.FormValue("has_nursery") == "on" {
			hasNursery = 1
		}
		hasHealing := 0
		if r.FormValue("has_healing") == "on" {
			hasHealing = 1
		}
		regionID := formInt(r.FormValue("region_id"), 1)
		_, err := db.DB.Exec(
			"UPDATE locations SET name = ?, description = ?, has_nursery = ?, has_healing = ?, region_id = ? WHERE id = ?",
			name, description, hasNursery, hasHealing, regionID, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations", http.StatusSeeOther)
	}
}

// удаление локации
func DeleteLocationHandler(w http.ResponseWriter, r *http.Request) {
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
	// сначала удаляем связанные записи (exits, npc) или полагаемся на каскадное удаление, если настроено
	_, _ = db.DB.Exec("DELETE FROM location_exits WHERE from_location = ? OR to_location = ?", id, id)
	_, _ = db.DB.Exec("DELETE FROM npc WHERE location_id = ?", id)
	_, err = db.DB.Exec("DELETE FROM locations WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/locations", http.StatusSeeOther)
}

// список регионов
func RegionsHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	rows, err := db.DB.Query(`
		SELECT r.id, r.name, COALESCE(r.current_weather_id, 1), COALESCE(w.name, ''),
		       (SELECT COUNT(*) FROM locations l WHERE l.region_id = r.id)
		FROM regions r
		LEFT JOIN weather w ON w.id = r.current_weather_id
		ORDER BY r.id
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Region struct {
		ID               int
		Name             string
		CurrentWeatherID int
		WeatherName      string
		LocationCount    int
	}
	var regions []Region
	for rows.Next() {
		var reg Region
		if err := rows.Scan(&reg.ID, &reg.Name, &reg.CurrentWeatherID, &reg.WeatherName, &reg.LocationCount); err == nil {
			regions = append(regions, reg)
		}
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/regions.html"))
	tmpl.Execute(w, regions)
}

// создание региона
func CreateRegionHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		weathers, err := loadWeatherOptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/region_form.html"))
		tmpl.Execute(w, regionFormData{CurrentWeatherID: 1, Weathers: weathers})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	weatherID := formInt(r.FormValue("current_weather_id"), 1)
	if _, err := db.DB.Exec("INSERT INTO regions (name, current_weather_id) VALUES (?, ?)", name, weatherID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/regions", http.StatusSeeOther)
}

// редактирование региона
func EditRegionHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id == 0 {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		var name string
		var weatherID int
		err := db.DB.QueryRow("SELECT name, COALESCE(current_weather_id, 1) FROM regions WHERE id = ?", id).Scan(&name, &weatherID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Region not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		weathers, err := loadWeatherOptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/region_form.html"))
		tmpl.Execute(w, regionFormData{ID: id, Name: name, CurrentWeatherID: weatherID, Weathers: weathers})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	weatherID := formInt(r.FormValue("current_weather_id"), 1)
	if _, err := db.DB.Exec("UPDATE regions SET name = ?, current_weather_id = ? WHERE id = ?", name, weatherID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/regions", http.StatusSeeOther)
}

// удаление региона (только если в нём нет локаций)
func DeleteRegionHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil || id == 0 {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	var count int
	db.DB.QueryRow("SELECT COUNT(*) FROM locations WHERE region_id = ?", id).Scan(&count)
	if count > 0 {
		http.Error(w, "Нельзя удалить регион, в котором есть локации", http.StatusConflict)
		return
	}
	if _, err := db.DB.Exec("DELETE FROM regions WHERE id = ?", id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/regions", http.StatusSeeOther)
}

// Добавление выхода (перехода) из локации
func AdminAddExitHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fromIDStr := r.FormValue("from_location")
	toIDStr := r.FormValue("to_location")
	if fromIDStr == "" || toIDStr == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	fromID, err := strconv.Atoi(fromIDStr)
	if err != nil {
		http.Error(w, "Invalid from_location", http.StatusBadRequest)
		return
	}
	toID, err := strconv.Atoi(toIDStr)
	if err != nil {
		http.Error(w, "Invalid to_location", http.StatusBadRequest)
		return
	}

	// Проверяем, не существует ли уже такой выход
	var count int
	db.DB.QueryRow("SELECT COUNT(*) FROM location_exits WHERE from_location = ? AND to_location = ?", fromID, toID).Scan(&count)
	if count > 0 {
		http.Error(w, "Exit already exists", http.StatusConflict)
		return
	}

	_, err = db.DB.Exec("INSERT INTO location_exits (from_location, to_location) VALUES (?, ?)", fromID, toID)
	if err != nil {
		http.Error(w, "Failed to add exit", http.StatusInternalServerError)
		return
	}

	// Возвращаемся на страницу редактирования локации
	http.Redirect(w, r, "/admin/locations/edit?id="+fromIDStr, http.StatusSeeOther)
}

// Удаление выхода (перехода)
func AdminRemoveExitHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fromIDStr := r.FormValue("from_location")
	toIDStr := r.FormValue("to_location")
	if fromIDStr == "" || toIDStr == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}
	fromID, err := strconv.Atoi(fromIDStr)
	if err != nil {
		http.Error(w, "Invalid from_location", http.StatusBadRequest)
		return
	}
	toID, err := strconv.Atoi(toIDStr)
	if err != nil {
		http.Error(w, "Invalid to_location", http.StatusBadRequest)
		return
	}

	_, err = db.DB.Exec("DELETE FROM location_exits WHERE from_location = ? AND to_location = ?", fromID, toID)
	if err != nil {
		http.Error(w, "Failed to remove exit", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/locations/edit?id="+fromIDStr, http.StatusSeeOther)
}

// Страница выдачи покемона (GET – форма, POST – сохранение)
// GivePokemonHandler – выдача покемона тренеру (админ-панель)
// GET – отображает форму выбора пользователя и покемона
// POST – создаёт запись в user_pokemons с расчётом статов
func GivePokemonHandler(w http.ResponseWriter, r *http.Request) {
	// Проверка прав администратора
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// ================= ОБРАБОТКА GET-ЗАПРОСА (форма) =================
	if r.Method == http.MethodGet {
		// Загружаем список пользователей
		usersRows, err := db.DB.Query("SELECT id, username FROM users ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer usersRows.Close()
		type User struct {
			ID   int
			Name string
		}
		var users []User
		for usersRows.Next() {
			var u User
			if err := usersRows.Scan(&u.ID, &u.Name); err == nil {
				users = append(users, u)
			}
		}

		// Загружаем список покемонов
		pokemonsRows, err := db.DB.Query("SELECT id, name FROM pokemons ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer pokemonsRows.Close()
		type Pokemon struct {
			ID   int
			Name string
		}
		var pokemons []Pokemon
		for pokemonsRows.Next() {
			var p Pokemon
			if err := pokemonsRows.Scan(&p.ID, &p.Name); err == nil {
				pokemons = append(pokemons, p)
			}
		}

		// Загружаем список натур
		natureRows, err := db.DB.Query("SELECT id, name, COALESCE(increased_stat,''), COALESCE(decreased_stat,'') FROM natures ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer natureRows.Close()
		type Nature struct {
			ID            int
			Name          string
			IncreasedStat string
			DecreasedStat string
		}
		var natures []Nature
		for natureRows.Next() {
			var n Nature
			if err := natureRows.Scan(&n.ID, &n.Name, &n.IncreasedStat, &n.DecreasedStat); err == nil {
				natures = append(natures, n)
			}
		}

		// Передаём данные в шаблон
		data := struct {
			Users    []User
			Pokemons []Pokemon
			Natures  []Nature
		}{users, pokemons, natures}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/give_pokemon.html"))
		tmpl.Execute(w, data)
		return
	}

	// ================= ОБРАБОТКА POST-ЗАПРОСА (сохранение) =================
	if r.Method == http.MethodPost {
		// Получаем ID пользователя и покемона из формы
		userID := r.FormValue("user_id")
		pokemonID := r.FormValue("pokemon_id")
		if userID == "" || pokemonID == "" {
			http.Error(w, "Missing user or pokemon", http.StatusBadRequest)
			return
		}
		uid, _ := strconv.Atoi(userID)
		pid, _ := strconv.Atoi(pokemonID)

		// ---------- 1. Параметры покемона ----------
		levelStr := r.FormValue("level")
		level := 1
		if levelStr != "" {
			if l, err := strconv.Atoi(levelStr); err == nil && l >= 1 && l <= 100 {
				level = l
			}
		}

		parseIV := func(name string) int {
			v := r.FormValue(name)
			if v == "" {
				return 0
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 31 {
				return 0
			}
			return n
		}
		iv := [6]int{
			parseIV("iv_hp"),
			parseIV("iv_attack"),
			parseIV("iv_defense"),
			parseIV("iv_speed"),
			parseIV("iv_special_attack"),
			parseIV("iv_special_defense"),
		}

		// ---------- 2. Характер (nature) ----------
		natureID, _ := strconv.Atoi(r.FormValue("nature_id"))
		if natureID < 1 {
			natureID = 1
		}

		// ---------- 3. Пол экземпляра (роллится ВНЕ единой точки) ----------
		// db.RollGender — единственный источник пола. Привлекательность и статы
		// считает единая точка utils.CreateUserPokemon.
		gender, err := db.RollGender(pid)
		if err != nil {
			// не валим выдачу из-за пола — по умолчанию бесполый
			gender = db.GenderNeutral
		}

		// ---------- 4. Создание экземпляра через ЕДИНУЮ точку ----------
		// (та же функция зовётся при ловле — utils.CreateUserPokemon).
		// ВНИМАНИЕ: помещается в первый свободный слот команды 1..6, иначе в
		// питомник (раньше admin всегда клал в питомник).
		userPokemonID, err := utils.CreateUserPokemon(uid, pid, level, iv, natureID, gender, 0, 0, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("Admin: created user_pokemon id=%d (pokemon_id=%d, user_id=%d, level=%d)", userPokemonID, pid, uid, level)
		// Перенаправляем обратно на страницу выдачи покемонов
		http.Redirect(w, r, "/admin/give_pokemon", http.StatusSeeOther)
	}
}
