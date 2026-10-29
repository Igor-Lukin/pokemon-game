package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"pokemon-game/server/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

const (
	evoConditionTimeDay   = "time_day"
	evoConditionTimeNight = "time_night"
	evoConditionAtkGtDef  = "atk_gt_def"
	evoConditionAtkLtDef  = "atk_lt_def"
	evoConditionAtkEqDef  = "atk_eq_def"
	evoConditionFairyMove = "fairy_move"
	// evoHappinessMax — порог friendship-эволюции: счастье должно достичь 255.
	evoHappinessMax = 255
)

// evolutionNow подменяется в тестах, чтобы день/ночь были детерминированы.
var evolutionNow = time.Now

// evolutionContext — всё, что нужно для проверки условий развилки на момент
// эволюции: рассчитанные Atk/Def, знание Fairy-атаки и текущее счастье.
type evolutionContext struct {
	level          int
	attack         int
	defense        int
	knowsFairyMove bool
	happiness      int
}

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

	return checkEvolutionFor(userPokemonID, pokemonID, level)
}

// evolutionTimeOfDay — серверное локальное время: 06:00–19:59 = день.
func evolutionTimeOfDay() string {
	if h := evolutionNow().Hour(); h >= 6 && h < 20 {
		return "day"
	}
	return "night"
}

// evolutionConditionMet проверяет одно условие развилки. Пустое условие = true
// (для level-эволюций условие не требуется). Неизвестный код = false, чтобы
// опечатка в админке не приводила к случайной эволюции.
func evolutionConditionMet(condition string, ctx *evolutionContext) bool {
	switch strings.TrimSpace(condition) {
	case "":
		return true
	case evoConditionTimeDay:
		return evolutionTimeOfDay() == "day"
	case evoConditionTimeNight:
		return evolutionTimeOfDay() == "night"
	case evoConditionAtkGtDef:
		return ctx.attack > ctx.defense
	case evoConditionAtkLtDef:
		return ctx.attack < ctx.defense
	case evoConditionAtkEqDef:
		return ctx.attack == ctx.defense
	case evoConditionFairyMove:
		return ctx.knowsFairyMove
	default:
		log.Printf("evolutionConditionMet: unknown condition %q", condition)
		return false
	}
}

func userPokemonKnowsMoveType(userPokemonID, typeID int) bool {
	var n int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM user_pokemon_learnable_attacks l
		JOIN attacks a ON a.id = l.attack_id
		WHERE l.user_pokemon_id = ? AND l.is_learned = 1 AND a.type_id = ?
	`, userPokemonID, typeID).Scan(&n)
	if err != nil {
		return false
	}
	return n > 0
}

func buildEvolutionContext(userPokemonID, pokemonID, level int) (*evolutionContext, error) {
	var natureID, happiness int
	var iv, ev [6]int
	err := db.DB.QueryRow(`
		SELECT nature_id, COALESCE(happiness, 0),
		       iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
		       ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense
		FROM user_pokemons WHERE id = ?
	`, userPokemonID).Scan(&natureID, &happiness,
		&iv[0], &iv[1], &iv[2], &iv[3], &iv[4], &iv[5],
		&ev[0], &ev[1], &ev[2], &ev[3], &ev[4], &ev[5])
	if err != nil {
		return nil, err
	}

	var base [6]int
	err = db.DB.QueryRow(`
		SELECT base_hp, base_attack, base_defense, base_speed,
		       base_special_attack, base_special_defense
		FROM pokemons WHERE id = ?
	`, pokemonID).Scan(&base[0], &base[1], &base[2], &base[3], &base[4], &base[5])
	if err != nil {
		return nil, err
	}

	natureFactors, err := utils.GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1, 1, 1, 1, 1, 1}
	}
	train := [6]float64{1, 1, 1, 1, 1, 1}

	_, _, atk, def, _, _, _ :=
		utils.CalculateStats(base, iv, ev, level, natureFactors, train)

	return &evolutionContext{
		level:          level,
		attack:         atk,
		defense:        def,
		knowsFairyMove: userPokemonKnowsMoveType(userPokemonID, 18),
		happiness:      happiness,
	}, nil
}

// checkEvolutionFor выбирает ОДНУ ветку детерминированно:
//   - level-метод (1): нужен достигнутый evolution_level + условие;
//   - friendship (4): нужно счастье >= 255 (плюс условие, если задано:
//     день/ночь для Эспеона/Умбреона/Фросмота, фейи-атака для Сильвеона);
//   - при нескольких подходящих — evolution_priority DESC, затем id ASC.
func checkEvolutionFor(userPokemonID, pokemonID, level int) (*EvolutionInfo, error) {
	ctx, err := buildEvolutionContext(userPokemonID, pokemonID, level)
	if err != nil {
		return nil, err
	}

	rows, err := db.DB.Query(`
		SELECT id, name, COALESCE(evolution_level, 0),
		       COALESCE(evolution_method_id, 0),
		       COALESCE(evolution_condition, ''),
		       COALESCE(evolution_priority, 0)
		FROM pokemons
		WHERE evolves_from_id = ? AND evolution_method_id IN (1, 4)
		ORDER BY COALESCE(evolution_priority, 0) DESC, id ASC
	`, pokemonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var evolvesToID, evolutionLevel, methodID, priority int
		var evolvesToName, condition string
		if err := rows.Scan(&evolvesToID, &evolvesToName, &evolutionLevel,
			&methodID, &condition, &priority); err != nil {
			return nil, err
		}

		if methodID == 1 && (evolutionLevel <= 0 || level < evolutionLevel) {
			continue
		}
		if methodID == 4 && ctx.happiness < evoHappinessMax {
			continue
		}
		if !evolutionConditionMet(condition, ctx) {
			continue
		}

		log.Printf("checkEvolution: pokemon_id=%d level=%d -> id=%d name=%s (method=%d condition=%q priority=%d)",
			pokemonID, level, evolvesToID, evolvesToName, methodID, condition, priority)
		return &EvolutionInfo{
			CanEvolve:      true,
			EvolvesToID:    evolvesToID,
			EvolvesToName:  evolvesToName,
			EvolutionLevel: evolutionLevel,
		}, nil
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &EvolutionInfo{CanEvolve: false}, nil
}

type evolvedStats struct {
	level          int
	maxHP          int
	attack         int
	defense        int
	speed          int
	specialAttack  int
	specialDefense int
	name           string
	imageURL       string
}

// calcEvolvedStats считает статы экземпляра после смены вида (уровень/IV/EV/
// характер сохраняются, база берётся у нового вида).
func calcEvolvedStats(userPokemonID, newPokemonID int) (*evolvedStats, error) {
	var level, natureID int
	var iv [6]int
	var ev [6]int
	err := db.DB.QueryRow(`
		SELECT level, nature_id,
		       iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
		       ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense
		FROM user_pokemons WHERE id = ?
	`, userPokemonID).Scan(&level, &natureID,
		&iv[0], &iv[1], &iv[2], &iv[3], &iv[4], &iv[5],
		&ev[0], &ev[1], &ev[2], &ev[3], &ev[4], &ev[5])
	if err != nil {
		return nil, err
	}

	var base [6]int
	var newName, newImageURL string
	err = db.DB.QueryRow(`
		SELECT base_hp, base_attack, base_defense, base_speed, base_special_attack, base_special_defense,
		       name, image_url
		FROM pokemons WHERE id = ?
	`, newPokemonID).Scan(&base[0], &base[1], &base[2], &base[3], &base[4], &base[5],
		&newName, &newImageURL)
	if err != nil {
		return nil, err
	}

	natureFactors, err := utils.GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1, 1, 1, 1, 1, 1}
	}
	train := [6]float64{1, 1, 1, 1, 1, 1}

	_, maxHP, atk, def, spd, spAtk, spDef :=
		utils.CalculateStats(base, iv, ev, level, natureFactors, train)

	return &evolvedStats{
		level:          level,
		maxHP:          maxHP,
		attack:         atk,
		defense:        def,
		speed:          spd,
		specialAttack:  spAtk,
		specialDefense: spDef,
		name:           newName,
		imageURL:       newImageURL,
	}, nil
}

// EvolveWithItem применяет предмет эволюции к покемону пользователя и атомарно
// списывает ровно один предмет. Если предмет не подходит виду — applied=false,
// сообщение для игрока, ничего не меняется и предмет не тратится.
func EvolveWithItem(userID, userPokemonID, itemID int) (bool, string, error) {
	var ownerID, oldPokemonID int
	if err := db.DB.QueryRow(
		"SELECT user_id, pokemon_id FROM user_pokemons WHERE id = ?",
		userPokemonID,
	).Scan(&ownerID, &oldPokemonID); err != nil {
		return false, "", err
	}
	if ownerID != userID {
		return false, "", errPermission
	}

	var newPokemonID int
	var newName string
	// Ключ item-эволюции — evolution_item_id, а не method: часть каменных
	// строк исторически лежит с method=1/2 или NULL, и фильтр по методу
	// ломал такие эволюции. Конкретный камень однозначно задаёт цель.
	err := db.DB.QueryRow(`
		SELECT id, name FROM pokemons
		WHERE evolves_from_id = ? AND evolution_item_id = ?
		ORDER BY id LIMIT 1
	`, oldPokemonID, itemID).Scan(&newPokemonID, &newName)
	if err == sql.ErrNoRows {
		return false, "Этот предмет не подходит этому покемону.", nil
	}
	if err != nil {
		return false, "", err
	}

	st, err := calcEvolvedStats(userPokemonID, newPokemonID)
	if err != nil {
		return false, "", err
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback()

	// Списываем предмет в той же транзакции, что и эволюцию: ровно один раз.
	res, err := tx.Exec(`
		UPDATE user_items SET quantity = quantity - 1
		WHERE user_id = ? AND item_id = ? AND quantity > 0
	`, userID, itemID)
	if err != nil {
		return false, "", err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, "", err
	}
	if affected == 0 {
		return false, "У вас нет этого предмета.", nil
	}
	if _, err := tx.Exec(
		"DELETE FROM user_items WHERE user_id = ? AND item_id = ? AND quantity <= 0",
		userID, itemID,
	); err != nil {
		return false, "", err
	}

	// Гард по старому виду: параллельный запрос не сможет эволюционировать дважды.
	res, err = tx.Exec(`
		UPDATE user_pokemons SET
			pokemon_id = ?,
			max_hp = ?, attack = ?, defense = ?, speed = ?,
			special_attack = ?, special_defense = ?,
			current_hp = ?
		WHERE id = ? AND user_id = ? AND pokemon_id = ?
	`, newPokemonID, st.maxHP, st.attack, st.defense, st.speed,
		st.specialAttack, st.specialDefense, st.maxHP,
		userPokemonID, userID, oldPokemonID)
	if err != nil {
		return false, "", err
	}
	affected, err = res.RowsAffected()
	if err != nil {
		return false, "", err
	}
	if affected == 0 {
		return false, "Покемон уже изменился, попробуйте снова.", nil
	}

	if err := tx.Commit(); err != nil {
		return false, "", err
	}

	if err := db.InitLearnableAttacks(userPokemonID, newPokemonID, st.level); err != nil {
		log.Printf("EvolveWithItem: InitLearnableAttacks error: %v", err)
	}
	if err := db.RemapAbilityOnEvolve(userPokemonID, oldPokemonID, newPokemonID); err != nil {
		log.Printf("EvolveWithItem: RemapAbilityOnEvolve error: %v", err)
	}

	log.Printf("User %d evolved pokemon %d to %s (pokemon_id %d) using item %d",
		userID, userPokemonID, newName, newPokemonID, itemID)

	return true, "Поздравляем! Ваш покемон эволюционировал в " + newName + "!", nil
}

// evolveByInfo применяет выбранную ветку эволюции к экземпляру: проверка
// владельца, пересчёт статов, смена вида (гард по старому pokemon_id),
// InitLearnableAttacks и RemapAbilityOnEvolve. Возвращает имя и image_url
// нового вида. Используется в т.ч. useHappiness при достижении счастья 255.
func evolveByInfo(userID, userPokemonID int, info *EvolutionInfo) (string, string, error) {
	var ownerID, oldPokemonID int
	if err := db.DB.QueryRow(
		"SELECT user_id, pokemon_id FROM user_pokemons WHERE id = ?",
		userPokemonID,
	).Scan(&ownerID, &oldPokemonID); err != nil {
		return "", "", err
	}
	if ownerID != userID {
		return "", "", errPermission
	}

	st, err := calcEvolvedStats(userPokemonID, info.EvolvesToID)
	if err != nil {
		return "", "", err
	}

	res, err := db.DB.Exec(`
		UPDATE user_pokemons SET
			pokemon_id = ?,
			max_hp = ?, attack = ?, defense = ?, speed = ?,
			special_attack = ?, special_defense = ?,
			current_hp = ?
		WHERE id = ? AND user_id = ? AND pokemon_id = ?
	`, info.EvolvesToID, st.maxHP, st.attack, st.defense, st.speed,
		st.specialAttack, st.specialDefense, st.maxHP,
		userPokemonID, userID, oldPokemonID)
	if err != nil {
		return "", "", err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return "", "", errors.New("покемон уже изменился, попробуйте снова")
	}

	if err := db.InitLearnableAttacks(userPokemonID, info.EvolvesToID, st.level); err != nil {
		log.Printf("evolveByInfo: InitLearnableAttacks error: %v", err)
	}
	if err := db.RemapAbilityOnEvolve(userPokemonID, oldPokemonID, info.EvolvesToID); err != nil {
		log.Printf("evolveByInfo: RemapAbilityOnEvolve error: %v", err)
	}

	log.Printf("User %d evolved pokemon %d to %s (pokemon_id %d) via info",
		userID, userPokemonID, st.name, info.EvolvesToID)
	return st.name, st.imageURL, nil
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

// GetEvolutionItemTargetsHandler отдаёт id покемонов команды, которые могут
// эволюционировать этим предметом. Нужен клиенту, чтобы в списке выбора камня
// показывать только подходящих покемонов, а не всех подряд.
func GetEvolutionItemTargetsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	itemID, err := strconv.Atoi(vars["item_id"])
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := db.DB.Query(`
		SELECT DISTINCT up.id
		FROM user_pokemons up
		JOIN pokemons p ON p.id = up.pokemon_id
		JOIN pokemons evo ON evo.evolves_from_id = p.id
		WHERE up.user_id = ? AND up.position BETWEEN 1 AND 6
		  AND evo.evolution_item_id = ?
		ORDER BY up.id
	`, userID, itemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ids = append(ids, id)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ids)
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

	// [ability] Старый вид прочитать ДО UPDATE pokemon_id — нужен для ремапа
	// способности по слоту (см. db.RemapAbilityOnEvolve).
	var oldPokemonID int
	if err := db.DB.QueryRow("SELECT pokemon_id FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&oldPokemonID); err != nil {
		http.Error(w, "Pokemon not found", http.StatusNotFound)
		return
	}

	st, err := calcEvolvedStats(userPokemonID, newPokemonID)
	if err != nil {
		http.Error(w, "Pokemon or evolution target not found", http.StatusNotFound)
		return
	}

	// Update user_pokemon
	_, err = db.DB.Exec(`
		UPDATE user_pokemons SET
			pokemon_id = ?,
			max_hp = ?, attack = ?, defense = ?, speed = ?,
			special_attack = ?, special_defense = ?,
			current_hp = ?
		WHERE id = ?
	`, newPokemonID, st.maxHP, st.attack, st.defense, st.speed,
		st.specialAttack, st.specialDefense, st.maxHP, userPokemonID)
	if err != nil {
		log.Printf("EvolveHandler: update error: %v", err)
		http.Error(w, "Failed to evolve", http.StatusInternalServerError)
		return
	}

	// Re-init learnable attacks for the new form
	if err := db.InitLearnableAttacks(userPokemonID, newPokemonID, st.level); err != nil {
		log.Printf("EvolveHandler: InitLearnableAttacks error: %v", err)
	}

	// [ability] Ремап способности по слоту (единая точка — см. §5.4 ТЗ).
	if err := db.RemapAbilityOnEvolve(userPokemonID, oldPokemonID, newPokemonID); err != nil {
		log.Printf("EvolveHandler: RemapAbilityOnEvolve error: %v", err)
	}

	log.Printf("User %d evolved pokemon %d to %s (pokemon_id %d)", userID, userPokemonID, st.name, newPokemonID)

	resp := EvolveResponse{
		Success:      true,
		NewPokemonID: newPokemonID,
		NewName:      st.name,
		NewImageURL:  st.imageURL,
		Message:      "Поздравляем! Ваш покемон эволюционировал в " + st.name + "!",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
