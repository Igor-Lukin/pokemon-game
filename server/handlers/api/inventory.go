package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"
)

var errPermission = errors.New("permission denied")

type UseItemRequest struct {
	ItemID        int  `json:"item_id"`
	UserPokemonID *int `json:"user_pokemon_id,omitempty"`
	Quantity      int  `json:"quantity"`
}

type UseItemResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func GetInventoryHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	inv, err := db.GetUserInventory(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inv)
}

func UseItemHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UseItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	item, err := db.GetItemByID(req.ItemID)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	inv, err := db.GetUserInventory(userID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	var available int
	for _, ui := range inv {
		if ui.ID == req.ItemID {
			available = ui.Quantity
			break
		}
	}
	if available <= 0 {
		http.Error(w, "У вас нет этого предмета", http.StatusConflict)
		return
	}
	if req.Quantity > available {
		req.Quantity = available
	}

	var message string
	applied := 1
	consumed := false

	switch item.EffectType {
	case "heal_hp":
		if req.UserPokemonID == nil {
			http.Error(w, "user_pokemon_id is required", http.StatusBadRequest)
			return
		}
		err = useHealHP(userID, *req.UserPokemonID, item.EffectValue)
	case "heal_status":
		if req.UserPokemonID == nil {
			http.Error(w, "user_pokemon_id is required", http.StatusBadRequest)
			return
		}
		err = useHealStatus(userID, *req.UserPokemonID, item.EffectValue)
	case "revive":
		if req.UserPokemonID == nil {
			http.Error(w, "user_pokemon_id is required", http.StatusBadRequest)
			return
		}
		err = useRevive(userID, *req.UserPokemonID, item.EffectValue)
	case "candy":
		if req.UserPokemonID == nil {
			http.Error(w, "user_pokemon_id is required", http.StatusBadRequest)
			return
		}
		message, applied, err = useCandy(userID, *req.UserPokemonID, req.Quantity, item.EffectValue)
	case "happiness":
		if req.UserPokemonID == nil {
			http.Error(w, "user_pokemon_id is required", http.StatusBadRequest)
			return
		}
		message, applied, err = useHappiness(userID, *req.UserPokemonID, item.EffectValue, req.Quantity)
	case "evolve":
		if req.UserPokemonID == nil {
			http.Error(w, "user_pokemon_id is required", http.StatusBadRequest)
			return
		}
		var evolved bool
		evolved, message, err = EvolveWithItem(userID, *req.UserPokemonID, req.ItemID)
		if err == nil && !evolved {
			// Неподходящий предмет — no-op: ничего не меняем и не списываем.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(UseItemResponse{Success: false, Message: message})
			return
		}
		if evolved {
			consumed = true
		}
	default:
		http.Error(w, "Этот предмет нельзя использовать напрямую", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !consumed {
		removed, err := db.RemoveItemFromUser(userID, req.ItemID, applied)
		if err != nil || !removed {
			log.Printf("UseItemHandler: failed to remove item %d from user %d: %v", req.ItemID, userID, err)
			http.Error(w, "Failed to consume item", http.StatusInternalServerError)
			return
		}
	}

	log.Printf("User %d used item %s (id=%d)", userID, item.Name, req.ItemID)

	if message == "" {
		message = "Предмет использован!"
	}
	resp := UseItemResponse{
		Success: true,
		Message: message,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func useHealHP(userID, userPokemonID, amount int) error {
	var ownerID int
	var currentHP, maxHP int
	err := db.DB.QueryRow("SELECT user_id, current_hp, max_hp FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&ownerID, &currentHP, &maxHP)
	if err != nil {
		return err
	}
	if ownerID != userID {
		return errPermission
	}

	newHP := currentHP + amount
	if amount >= 9999 || newHP > maxHP {
		newHP = maxHP
	}
	_, err = db.DB.Exec("UPDATE user_pokemons SET current_hp = ? WHERE id = ?", newHP, userPokemonID)
	return err
}

func useHealStatus(userID, userPokemonID, statusID int) error {
	var ownerID int
	err := db.DB.QueryRow("SELECT user_id FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&ownerID)
	if err != nil {
		return err
	}
	if ownerID != userID {
		return errPermission
	}

	if statusID == 0 {
		_, err = db.DB.Exec("UPDATE user_pokemons SET status_id = 1 WHERE id = ?", userPokemonID)
		return err
	}
	_, err = db.DB.Exec("UPDATE user_pokemons SET status_id = 1 WHERE id = ? AND status_id = ?", userPokemonID, statusID)
	return err
}

func useRevive(userID, userPokemonID, percent int) error {
	var ownerID int
	var currentHP, maxHP int
	err := db.DB.QueryRow("SELECT user_id, current_hp, max_hp FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&ownerID, &currentHP, &maxHP)
	if err != nil {
		return err
	}
	if ownerID != userID {
		return errPermission
	}
	if currentHP > 0 {
		return nil
	}

	newHP := maxHP * percent / 100
	if newHP < 1 {
		newHP = 1
	}
	if percent >= 100 {
		newHP = maxHP
	}
	_, err = db.DB.Exec("UPDATE user_pokemons SET current_hp = ?, status_id = 1 WHERE id = ?", newHP, userPokemonID)
	return err
}

// useCandy поднимает уровень покемона на quantity и начисляет evPerLevel
// нераспределённых EV за каждый новый уровень (effect_value предмета:
// «Леденец» = 2, «Жёлтая конфета» = 3).
func useCandy(userID, userPokemonID, quantity, evPerLevel int) (string, int, error) {
	if evPerLevel <= 0 {
		evPerLevel = 2
	}
	var ownerID, level, pokemonID, natureID, availableEV int
	var iv [6]int
	var ev [6]int
	err := db.DB.QueryRow(`
		SELECT user_id, level, pokemon_id, nature_id,
		       iv_hp, iv_attack, iv_defense, iv_speed,
		       iv_special_attack, iv_special_defense,
		       ev_hp, ev_attack, ev_defense, ev_speed,
		       ev_special_attack, ev_special_defense,
		       COALESCE(available_ev, 0)
		FROM user_pokemons WHERE id = ?
	`, userPokemonID).Scan(&ownerID, &level, &pokemonID, &natureID,
		&iv[0], &iv[1], &iv[2], &iv[3], &iv[4], &iv[5],
		&ev[0], &ev[1], &ev[2], &ev[3], &ev[4], &ev[5], &availableEV)
	if err != nil {
		return "", 0, err
	}
	if ownerID != userID {
		return "", 0, errPermission
	}

	applied := 0
	for i := 0; i < quantity; i++ {
		if level >= 100 {
			break
		}
		level++
		availableEV += evPerLevel
		applied++
	}

	if applied == 0 {
		return "", 0, errors.New("Покемон уже достиг максимального уровня")
	}

	newPokemonID, evoInfo, _, err := applyLevelUp(userPokemonID, pokemonID, level, iv, ev, natureID, availableEV)
	if err != nil {
		return "", 0, err
	}

	log.Printf("User %d used %d candy on pokemon %d -> level %d (pokemon_id %d)", userID, applied, userPokemonID, level, newPokemonID)

	msg := "Покемон повысил уровень до " + strconv.Itoa(level) + "! (+" +
		strconv.Itoa(applied*evPerLevel) + " EV)"
	if evoInfo != nil && evoInfo.CanEvolve {
		msg += " " + evoInfo.EvolvesToName + " ждёт эволюции! Зайдите в профиль покемона."
	}
	return msg, applied, nil
}

// useHappiness поднимает счастье покемона на amount за каждую использованную
// единицу предмета (кламп на 255). При достижении 255 сразу проверяет и
// применяет friendship-эволюцию, чтобы покемон эволюционировал в момент
// максимума счастья, а не только на следующем уровне.
func useHappiness(userID, userPokemonID, amount, quantity int) (string, int, error) {
	if amount <= 0 {
		amount = 1
	}

	var ownerID, happiness, pokemonID, level int
	err := db.DB.QueryRow(`
		SELECT user_id, COALESCE(happiness, 0), pokemon_id, level
		FROM user_pokemons WHERE id = ?
	`, userPokemonID).Scan(&ownerID, &happiness, &pokemonID, &level)
	if err != nil {
		return "", 0, err
	}
	if ownerID != userID {
		return "", 0, errPermission
	}
	if happiness >= evoHappinessMax {
		return "", 0, errors.New("Счастье уже максимальное")
	}

	before := happiness
	applied := 0
	for i := 0; i < quantity; i++ {
		if happiness >= evoHappinessMax {
			break
		}
		happiness += amount
		if happiness > evoHappinessMax {
			happiness = evoHappinessMax
		}
		applied++
	}

	if _, err := db.DB.Exec(
		"UPDATE user_pokemons SET happiness = ? WHERE id = ?",
		happiness, userPokemonID,
	); err != nil {
		return "", 0, err
	}

	msg := "Счастье: " + strconv.Itoa(happiness) + "/" + strconv.Itoa(evoHappinessMax) +
		" (+" + strconv.Itoa(happiness-before) + ")"

	// Достигли порога — пытаемся эволюционировать сразу (дружба).
	if happiness >= evoHappinessMax {
		info, err := checkEvolutionFor(userPokemonID, pokemonID, level)
		if err != nil {
			log.Printf("useHappiness: checkEvolutionFor error: %v", err)
		} else if info != nil && info.CanEvolve {
			name, _, err := evolveByInfo(userID, userPokemonID, info)
			if err != nil {
				log.Printf("useHappiness: evolveByInfo error: %v", err)
			} else {
				msg += " " + name + " эволюционировал!"
			}
		}
	}

	log.Printf("User %d used %d happiness item on pokemon %d: happiness %d -> %d",
		userID, applied, userPokemonID, before, happiness)
	return msg, applied, nil
}
