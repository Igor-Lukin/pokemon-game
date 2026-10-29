package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	mathrand "math/rand"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"pokemon-game/server/utils"
	"time"
)

// breeding.go — РАЗВЕДЕНИЕ ПОКЕМОНОВ (поток [13]).
//
// Пара из питомника -> ЯЙЦО (вид = базовая форма линии, IV = макс родителей +30%)
// -> ИНКУБАЦИЯ по таймеру -> вылупление в потомка через utils.CreateUserPokemon.
//
// Правило совместимости пары живёт в db.BreedCompat (один вид, одна буква, M/F
// либо N/N с Амуритом на обоих). Единая точка создания экземпляра —
// utils.CreateUserPokemon (роллит букву/статы/атаки; сюда не дублируем).

// ------------------------- КОНСТАНТЫ-«РУЧКИ» -------------------------
// Их крутит владелец. Для быстрой проверки таймера можно ВРЕМЕННО поставить
// incubationDuration маленьким (напр. 10*time.Second) и вернуть 1 час перед сдачей.
const (
	incubationDuration = 1 * time.Hour // сколько зреет яйцо
	hatchLevel         = 1             // уровень вылупившегося потомка
	ivPlusChance       = 0.30          // шанс +1 к унаследованному гену
	ivCap              = 31            // потолок IV
	baseFormMaxSteps   = 10            // защита от цикла в линии эволюции
	maxEggsPerUser     = 3             // лимит незавершённых яиц на игрока
)

// eggTimeLayout — формат хранения created_at/hatch_at в таблице eggs
// (UTC, тот же вид, что даёт SQLite CURRENT_TIMESTAMP). Источник правды по
// готовности — серверное время: ready = now(UTC) >= hatch_at.
const eggTimeLayout = "2006-01-02 15:04:05"

// breedParent — слепок родителя для проверки и расчёта потомка.
type breedParent struct {
	up       db.UserPokemon
	position int
	iv       [6]int // HP,Atk,Def,Spd,SpAtk,SpDef
}

// loadBreedParent читает экземпляр игрока с надетым эффектом и IV.
// sql.ErrNoRows -> покемон не найден/не принадлежит игроку.
func loadBreedParent(userID, id int) (*breedParent, error) {
	var p breedParent
	var gender, attr, heldEffect sql.NullString
	var pos sql.NullInt64
	err := db.DB.QueryRow(`
		SELECT up.pokemon_id, up.gender, up.attractiveness, up.position,
		       COALESCE(it.effect_type, ''),
		       up.iv_hp, up.iv_attack, up.iv_defense, up.iv_speed,
		       up.iv_special_attack, up.iv_special_defense
		FROM user_pokemons up
		LEFT JOIN items it ON up.held_item_id = it.id
		WHERE up.id = ? AND up.user_id = ?`, id, userID).Scan(
		&p.up.PokemonID, &gender, &attr, &pos, &heldEffect,
		&p.iv[0], &p.iv[1], &p.iv[2], &p.iv[3], &p.iv[4], &p.iv[5])
	if err != nil {
		return nil, err
	}
	p.up.ID = id
	p.up.Gender = gender.String
	p.up.Attractiveness = attr.String
	p.up.HeldEffect = heldEffect.String
	p.position = int(pos.Int64) // NULL -> 0 (питомник)
	return &p, nil
}

// baseFormOf спускается по evolves_from_id до начальной формы линии.
// Защита от цикла в данных: не более baseFormMaxSteps шагов и по посещённым id.
func baseFormOf(pokemonID int) int {
	cur := pokemonID
	visited := map[int]bool{}
	for step := 0; step < baseFormMaxSteps; step++ {
		if visited[cur] {
			break
		}
		visited[cur] = true
		var from sql.NullInt64
		err := db.DB.QueryRow("SELECT evolves_from_id FROM pokemons WHERE id = ?", cur).Scan(&from)
		if err != nil || !from.Valid || from.Int64 == 0 {
			break
		}
		cur = int(from.Int64)
	}
	return cur
}

// childIV — наследование IV: для каждого гена max(p1,p2), затем с шансом
// ivPlusChance +1 (кап ivCap). Наследуем ТОЛЬКО IV (натура/буква/пол — случайные).
func childIV(a, b [6]int) [6]int {
	var out [6]int
	for i := 0; i < 6; i++ {
		v := a[i]
		if b[i] > v {
			v = b[i]
		}
		if v < ivCap && mathrand.Float64() < ivPlusChance {
			v++
		}
		if v > ivCap {
			v = ivCap
		}
		out[i] = v
	}
	return out
}

// ------------------------------ ХЕНДЛЕРЫ ------------------------------

type breedRequest struct {
	Parent1ID int `json:"parent1_id"`
	Parent2ID int `json:"parent2_id"`
}

type eggJSON struct {
	ID               int      `json:"id"`
	SpeciesID        int      `json:"species_id"`
	Name             string   `json:"name"`
	ImageURL         string   `json:"image_url"`
	HatchAt          string   `json:"hatch_at"`
	RemainingSeconds int      `json:"remaining_seconds"`
	Ready            bool     `json:"ready"`
	Evolved          []string `json:"evolved,omitempty"`
}

// BreedHandler — POST /api/nursery/breed {parent1_id, parent2_id}. Сводит пару
// из питомника и создаёт яйцо.
func BreedHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req breedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.Parent1ID == req.Parent2ID {
		http.Error(w, "Нужны два разных покемона", http.StatusBadRequest)
		return
	}

	p1, err := loadBreedParent(userID, req.Parent1ID)
	if err == sql.ErrNoRows {
		http.Error(w, "Покемон не найден", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	p2, err := loadBreedParent(userID, req.Parent2ID)
	if err == sql.ErrNoRows {
		http.Error(w, "Покемон не найден", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// Оба должны быть в ПИТОМНИКЕ (position 0/NULL).
	if p1.position != 0 || p2.position != 0 {
		http.Error(w, "Разводить можно только покемонов из питомника", http.StatusBadRequest)
		return
	}

	if okCompat, reason := db.BreedCompat(&p1.up, &p2.up); !okCompat {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}

	// Одно незавершённое яйцо на пару проверяется в транзакции ниже.

	speciesID := baseFormOf(p1.up.PokemonID) // виды равны -> базовая форма общего вида
	iv := childIV(p1.iv, p2.iv)

	// Лимит яиц + проверка дубликата пары + INSERT — в одной транзакции
	// (BEGIN IMMEDIATE), чтобы двойной сабмит не создал два яйца.
	ctx := r.Context()
	conn, err := db.DB.Conn(ctx)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	rollback := true
	defer func() {
		if rollback {
			conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		log.Printf("BreedHandler: begin: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	var eggCount int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM eggs WHERE user_id = ?", userID).Scan(&eggCount); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if eggCount >= maxEggsPerUser {
		http.Error(w, "Инкубатор полон (3 яйца)", http.StatusBadRequest)
		return
	}

	// Одно незавершённое яйцо на пару (в любом порядке родителей).
	var dup int
	if err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM eggs
		WHERE user_id = ?
		  AND ((parent1_id = ? AND parent2_id = ?) OR (parent1_id = ? AND parent2_id = ?))`,
		userID, p1.up.ID, p2.up.ID, p2.up.ID, p1.up.ID).Scan(&dup); err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if dup > 0 {
		http.Error(w, "У этой пары уже есть яйцо", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	hatch := now.Add(incubationDuration)
	res, err := conn.ExecContext(ctx, `
		INSERT INTO eggs (user_id, species_id, iv_hp, iv_attack, iv_defense,
			iv_speed, iv_special_attack, iv_special_defense,
			parent1_id, parent2_id, created_at, hatch_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, speciesID, iv[0], iv[1], iv[2], iv[3], iv[4], iv[5],
		p1.up.ID, p2.up.ID, now.Format(eggTimeLayout), hatch.Format(eggTimeLayout))
	if err != nil {
		log.Printf("BreedHandler: insert egg: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	eggID, _ := res.LastInsertId()

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		log.Printf("BreedHandler: commit: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	rollback = false

	var name string
	var img sql.NullString
	db.DB.QueryRow("SELECT name, image_url FROM pokemons WHERE id = ?", speciesID).Scan(&name, &img)

	remaining := int(time.Until(hatch).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	log.Printf("User %d bred pokemons %d+%d -> egg %d (species %d)", userID, p1.up.ID, p2.up.ID, eggID, speciesID)

	// [эволюция] «Разведение» (бывший «обмен», method=2): после успешного
	// спаривания эволюционирует каждый родитель, у вида которого есть такая
	// эволюция. Предметы (King's Rock/Metal Coat и пр.) для этого не нужны.
	var evolved []string
	for _, parentID := range []int{p1.up.ID, p2.up.ID} {
		if name, ok := evolveByBreeding(userID, parentID); ok {
			evolved = append(evolved, name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(eggJSON{
		ID: int(eggID), SpeciesID: speciesID, Name: name, ImageURL: img.String,
		HatchAt: hatch.Format(eggTimeLayout), RemainingSeconds: remaining, Ready: remaining == 0,
		Evolved: evolved,
	})
}

// evolveByBreeding эволюционирует покемона-родителя, если у его вида есть
// эволюция методом 2 («Разведение», исторически «обмен»). Возвращает имя
// новой формы и признак успеха.
func evolveByBreeding(userID, userPokemonID int) (string, bool) {
	var pokemonID int
	if err := db.DB.QueryRow(
		"SELECT pokemon_id FROM user_pokemons WHERE id = ? AND user_id = ?",
		userPokemonID, userID,
	).Scan(&pokemonID); err != nil {
		return "", false
	}
	var newID int
	var newName string
	err := db.DB.QueryRow(`
		SELECT id, name FROM pokemons
		WHERE evolves_from_id = ? AND evolution_method_id = 2
		ORDER BY id LIMIT 1`, pokemonID).Scan(&newID, &newName)
	if err != nil {
		return "", false
	}
	info := &EvolutionInfo{CanEvolve: true, EvolvesToID: newID, EvolvesToName: newName}
	evolvedName, _, err := evolveByInfo(userID, userPokemonID, info)
	if err != nil {
		log.Printf("evolveByBreeding: user_pokemon=%d -> %d: %v", userPokemonID, newID, err)
		return "", false
	}
	log.Printf("User %d: breeding evolution %d -> %s (pokemon_id %d)", userID, userPokemonID, evolvedName, newID)
	return evolvedName, true
}

// EggsHandler — GET /api/nursery/eggs. Список яиц игрока с обратным таймером.
func EggsHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := db.DB.Query(`
		SELECT e.id, e.species_id, p.name, COALESCE(p.image_url, ''), e.hatch_at
		FROM eggs e
		JOIN pokemons p ON e.species_id = p.id
		WHERE e.user_id = ?
		ORDER BY e.created_at`, userID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	list := make([]eggJSON, 0)
	for rows.Next() {
		var e eggJSON
		var hatchStr string
		if err := rows.Scan(&e.ID, &e.SpeciesID, &e.Name, &e.ImageURL, &hatchStr); err != nil {
			continue
		}
		e.HatchAt = hatchStr
		hatch, perr := time.Parse(eggTimeLayout, hatchStr)
		if perr != nil {
			e.Ready = true // не смогли разобрать время — не блокируем игрока
		} else {
			rem := int(hatch.Sub(now).Seconds())
			if rem < 0 {
				rem = 0
			}
			e.RemainingSeconds = rem
			e.Ready = rem == 0
		}
		list = append(list, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

type hatchRequest struct {
	EggID int `json:"egg_id"`
}

type hatchResponse struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Level int    `json:"level"`
}

// HatchHandler — POST /api/nursery/hatch {egg_id}. Вылупляет готовое яйцо
// в потомка через utils.CreateUserPokemon и удаляет яйцо.
func HatchHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req hatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var speciesID int
	var iv [6]int
	var hatchStr string
	err := db.DB.QueryRow(`
		SELECT species_id, iv_hp, iv_attack, iv_defense, iv_speed,
		       iv_special_attack, iv_special_defense, hatch_at
		FROM eggs WHERE id = ? AND user_id = ?`, req.EggID, userID).Scan(
		&speciesID, &iv[0], &iv[1], &iv[2], &iv[3], &iv[4], &iv[5], &hatchStr)
	if err == sql.ErrNoRows {
		http.Error(w, "Яйцо не найдено", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// Не раньше срока (серверное время — источник правды).
	if hatch, perr := time.Parse(eggTimeLayout, hatchStr); perr == nil {
		if time.Now().UTC().Before(hatch) {
			http.Error(w, "Яйцо ещё не готово", http.StatusBadRequest)
			return
		}
	}

	// Случайная натура (как в wild.go).
	var natureID int
	if err := db.DB.QueryRow("SELECT id FROM natures ORDER BY RANDOM() LIMIT 1").Scan(&natureID); err != nil {
		natureID = 13 // fallback: нейтральная
	}

	gender, err := db.RollGender(speciesID) // пол потомка по ратио его вида
	if err != nil {
		gender = db.GenderNeutral
	}

	newID, err := utils.CreateUserPokemon(userID, speciesID, hatchLevel, iv, natureID, gender, 0, 0, false)
	if err != nil {
		log.Printf("HatchHandler: CreateUserPokemon failed: %v", err)
		http.Error(w, "Не удалось создать потомка", http.StatusInternalServerError)
		return
	}

	db.DB.Exec("DELETE FROM eggs WHERE id = ? AND user_id = ?", req.EggID, userID)

	var name string
	db.DB.QueryRow("SELECT name FROM pokemons WHERE id = ?", speciesID).Scan(&name)

	log.Printf("User %d hatched egg %d -> pokemon %d (%s)", userID, req.EggID, newID, name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hatchResponse{ID: newID, Name: name, Level: hatchLevel})
}
