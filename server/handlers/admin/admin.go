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
	rows, err := db.DB.Query("SELECT id, name, description, has_nursery, has_healing FROM locations ORDER BY id")
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
	}
	var locations []Location
	for rows.Next() {
		var loc Location
		var nursery, healing int
		if err := rows.Scan(&loc.ID, &loc.Name, &loc.Description, &nursery, &healing); err != nil {
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
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/location_form.html"))
		tmpl.Execute(w, nil)
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
		_, err := db.DB.Exec("INSERT INTO locations (name, description, has_nursery, has_healing) VALUES (?, ?, ?, ?)", name, description, hasNursery, hasHealing)
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
		var hasNursery, hasHealing int
		err := db.DB.QueryRow("SELECT name, description, has_nursery, has_healing FROM locations WHERE id = ?", id).Scan(&name, &description, &hasNursery, &hasHealing)
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
		type Exit struct {
			ID   int
			Name string
		}
		var exits []Exit
		for rows.Next() {
			var e Exit
			if err := rows.Scan(&e.ID, &e.Name); err == nil {
				exits = append(exits, e)
			}
		}

		// Получаем список всех локаций для выпадающего списка добавления выхода
		allRows, err := db.DB.Query("SELECT id, name FROM locations ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allRows.Close()
		var allLocations []Exit
		for allRows.Next() {
			var loc Exit
			if err := allRows.Scan(&loc.ID, &loc.Name); err == nil {
				allLocations = append(allLocations, loc)
			}
		}

		data := struct {
			ID           int
			Name         string
			Description  string
			HasNursery   bool
			HasHealing   bool
			Exits        []Exit
			AllLocations []Exit
		}{ID: id, Name: name, Description: description, HasNursery: hasNursery == 1, HasHealing: hasHealing == 1, Exits: exits, AllLocations: allLocations}

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
		_, err := db.DB.Exec("UPDATE locations SET name = ?, description = ?, has_nursery = ?, has_healing = ? WHERE id = ?", name, description, hasNursery, hasHealing, id)
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

		// ---------- 1. Получаем базовые статы покемона ----------
		var base [6]int // порядок: HP, Attack, Defense, Speed, SpecialAttack, SpecialDefense
		err := db.DB.QueryRow(`
            SELECT base_hp, base_attack, base_defense, base_speed,
                   base_special_attack, base_special_defense
            FROM pokemons WHERE id = ?`, pid).Scan(
			&base[0], &base[1], &base[2], &base[3], &base[4], &base[5])
		if err != nil {
			http.Error(w, "Pokemon not found", http.StatusNotFound)
			return
		}

		// ---------- 2. Параметры покемона ----------
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
		// EV – все 0
		ev := [6]int{0, 0, 0, 0, 0, 0}
		// Количество свободных EV (нераспределённых) – 0
		availableEV := 0

		// ---------- 3. Множители характера (nature) ----------
		natureID, _ := strconv.Atoi(r.FormValue("nature_id"))
		if natureID < 1 {
			natureID = 1
		}
		var natureFactors [6]float64
		// Функция getNatureFactors (может быть определена в этом же файле или в utils)
		natureFactors, err = utils.GetNatureFactors(natureID)
		if err != nil {
			// Если не удалось загрузить – используем нейтральные 1.0
			natureFactors = [6]float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0}
		}

		// ---------- 4. Множители тренировки (train) – пока все 1.0 ----------
		train := [6]float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0}

		// ---------- 5. Вычисляем статы с помощью функции из utils ----------
		currentHP, maxHP, attack, defense, speed, specialAttack, specialDefense :=
			utils.CalculateStats(base, iv, ev, level, natureFactors, train)

		// ---------- 6. Вставляем запись в таблицу user_pokemons ----------
		// Для статуса и характера используем id=1 (healthy, hardy)
		result, err := db.DB.Exec(`
            INSERT INTO user_pokemons (
                user_id, pokemon_id, level, experience,
                current_hp, max_hp, attack, defense, speed,
                special_attack, special_defense,
                iv_hp, iv_attack, iv_defense, iv_speed,
                iv_special_attack, iv_special_defense,
                ev_hp, ev_attack, ev_defense, ev_speed,
                ev_special_attack, ev_special_defense,
                available_ev, position, status_id, nature_id
            ) VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 1, ?)`,
			uid, pid, level, currentHP, maxHP, attack, defense, speed, specialAttack, specialDefense,
			iv[0], iv[1], iv[2], iv[3], iv[4], iv[5],
			ev[0], ev[1], ev[2], ev[3], ev[4], ev[5],
			availableEV, natureID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		userPokemonID, _ := result.LastInsertId()
		log.Printf("Admin: created user_pokemon id=%d (pokemon_id=%d, user_id=%d, level=%d)", userPokemonID, pid, uid, level)
		if err := db.InitLearnableAttacks(int(userPokemonID), pid, level); err != nil {
			log.Printf("InitLearnableAttacks error: %v", err)
		}
		// Перенаправляем обратно на страницу выдачи покемонов
		http.Redirect(w, r, "/admin/give_pokemon", http.StatusSeeOther)
	}
}
