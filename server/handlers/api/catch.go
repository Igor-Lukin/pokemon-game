package api

import (
	"encoding/json"
	"log"
	mathrand "math/rand"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"pokemon-game/server/utils"
)

// catch.go — ЛОВЛЯ ДИКИХ покеболами (фаза 1: плоский множитель бола).
// Схема сделана так, чтобы фаза 2 (условные болы) добавилась без ломки:
// формула вынесена в catchChance, множитель берётся из items.catch_multiplier.

type CatchRequest struct {
	BattleID string `json:"battle_id"`
	ItemID   int    `json:"item_id"`
}

// catchChance — формула поимки фазы 1 (без «покачиваний»). Возвращает
// значение a: если a >= 255 — гарантированная поимка, иначе шанс = a/255.
// Вынесена отдельно, чтобы фаза 2 переиспользовала её для условных болов.
//
//	maxHP, hp   — макс. и текущее HP дикого;
//	baseRate    — pokemons.catch_rate (0..255, больше — легче);
//	ballMult100 — items.catch_multiplier (x100: обычный=100, Great=150, ...);
//	statusID    — статус дикого (0 = нет).
func catchChance(maxHP, hp, baseRate, ballMult100, statusID int) float64 {
	if maxHP <= 0 {
		maxHP = 1
	}
	if hp < 0 {
		hp = 0
	}
	ballMult := float64(ballMult100) / 100.0

	// statusMult: сон(5)/заморозка(4) = 2.5; паралич(2)/ожог(3)/яд(6,7) = 1.5.
	statusMult := 1.0
	switch statusID {
	case 4, 5:
		statusMult = 2.5
	case 2, 3, 6, 7:
		statusMult = 1.5
	}

	a := (float64(3*maxHP-2*hp) * float64(baseRate) * ballMult * statusMult) / float64(3*maxHP)
	return a
}

// ballMultiplierFor — эффективный множитель бола (x100) с учётом условия (фаза 2).
// Безусловный бол — множитель всегда. Условный — множитель при выполнении
// условия, иначе ×1 (100). active — активный покемон игрока (для Лавбола).
func ballMultiplierFor(it *db.Item, wild, active *BattlePokemon) int {
	base := it.CatchMultiplier
	if base <= 0 {
		base = 100
	}
	switch it.CatchCondition {
	case "type":
		if wild.PrimaryTypeID == it.CatchCondValue ||
			(wild.SecondaryTypeID != nil && *wild.SecondaryTypeID == it.CatchCondValue) {
			return base
		}
		return 100
	case "speed_gt":
		if wild.Speed > it.CatchCondValue {
			return base
		}
		return 100
	case "level_lt":
		if wild.Level < it.CatchCondValue {
			return base
		}
		return 100
	case "opposite_gender":
		if active != nil && wild.Gender != db.GenderNeutral &&
			active.Gender != db.GenderNeutral && wild.Gender != active.Gender {
			return base
		}
		return 100
	default: // '' — безусловный
		return base
	}
}

func CatchHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Всё чтение/запись battleSessions — под battleMu, до конца обработки.
	battleMu.Lock()
	defer battleMu.Unlock()

	bs, exists := battleSessions[req.BattleID]
	if !exists {
		http.Error(w, "Battle not found", http.StatusNotFound)
		return
	}
	if bs.BattleType != "wild" {
		http.Error(w, "Ловить можно только диких покемонов", http.StatusBadRequest)
		return
	}
	if bs.Finished || bs.WildPokemon == nil || bs.WildPokemon.IsFainted {
		http.Error(w, "Дикий покемон уже повержен", http.StatusBadRequest)
		return
	}

	// Проверка: предмет есть у игрока и это БОЛ (категория 1 или catch_bonus).
	inv, err := db.GetUserInventory(userID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	var owned *db.UserItem
	for i := range inv {
		if inv[i].ID == req.ItemID {
			owned = &inv[i]
			break
		}
	}
	if owned == nil || owned.Quantity <= 0 {
		http.Error(w, "У вас нет этого предмета", http.StatusConflict)
		return
	}
	if owned.CategoryID != 1 && owned.EffectType != "catch_bonus" {
		http.Error(w, "Это не покебол", http.StatusBadRequest)
		return
	}

	// Бол тратится СРАЗУ — и при провале тоже (списываем до броска).
	removed, err := db.RemoveItemFromUser(userID, req.ItemID, 1)
	if err != nil || !removed {
		log.Printf("CatchHandler: failed to remove ball %d from user %d: %v", req.ItemID, userID, err)
		http.Error(w, "Не удалось потратить покебол", http.StatusInternalServerError)
		return
	}

	wild := bs.WildPokemon
	active := &bs.PlayerTeam[bs.ActiveIdx]

	// Множитель бола (x100) с учётом условия (тип/скорость/уровень/пол).
	ballMult := ballMultiplierFor(&owned.Item, wild, active)

	// Базовый шанс вида.
	baseRate := 45
	db.DB.QueryRow("SELECT COALESCE(catch_rate, 45) FROM pokemons WHERE id = ?",
		wild.PokemonID).Scan(&baseRate)

	a := catchChance(wild.MaxHP, wild.CurrentHP, baseRate, ballMult, wild.StatusID)

	caught := false
	if a >= 255 {
		caught = true
	} else {
		caught = mathrand.Float64() < a/255.0
	}

	ballName := owned.Name

	// ------------------------------ УСПЕХ ------------------------------
	if caught {
		newID, err := utils.CreateUserPokemon(userID, wild.PokemonID, wild.Level,
			bs.WildIV, bs.WildNatureID, wild.Gender,
			owned.GeneBonus, owned.HappinessBonus, owned.ForceNursery != 0)
		if err != nil {
			log.Printf("CatchHandler: CreateUserPokemon failed: %v", err)
			http.Error(w, "Пойман, но не удалось сохранить покемона", http.StatusInternalServerError)
			return
		}

		// Ушёл ли в питомник (Трансбол принудительно или команда полна)?
		var pos int
		db.DB.QueryRow("SELECT position FROM user_pokemons WHERE id = ?", newID).Scan(&pos)
		toNursery := pos == 0

		bs.Finished = true
		delete(battleSessions, req.BattleID)

		msg := wild.Name + " пойман!"
		if toNursery {
			if owned.ForceNursery != 0 {
				msg += " Отправлен в питомник."
			} else {
				msg += " Команда полна — отправлен в питомник."
			}
		}
		log.Printf("User %d caught pokemon %d (%s) with %s -> user_pokemon %d (nursery=%v)",
			userID, wild.PokemonID, wild.Name, ballName, newID, toNursery)

		resp := BattleResponse{
			Type: "catch_success",
			Log: []BattleLogEntry{{
				Round:        bs.Turn,
				PokemonName:  wild.Name,
				PokemonImage: wild.ImageURL,
				Message:      msg,
			}},
			Wild: copyBattlePokemon(wild),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// ------------------------------ ПРОВАЛ ------------------------------
	// Дикий делает свой ход — переиспользуем ту же turn-логику, что после
	// смены покемона (SwitchHandler): движок НЕ дублируем.
	bs.Turn++ // ловля стоит хода игрока (как смена)
	logEntries := []BattleLogEntry{{
		Round:        bs.Turn,
		PokemonName:  wild.Name,
		PokemonImage: wild.ImageURL,
		Message:      wild.Name + " вырвался из " + ballName + "!",
	}}

	active = &bs.PlayerTeam[bs.ActiveIdx]

	if !wild.IsFainted {
		wildMove := chooseWildMoveOrCharge(wild)
		if wildMove != nil {
			if wildMove.CategoryID != 3 && wildMove.Power > 0 {
				fainted, wc := executeDamagingMove(bs, wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries, sideFor(bs, active), sideFor(bs, wild), bs.WaterSportTurns > 0)
				if wc > 0 {
					setBattleWeather(bs, wc, &logEntries)
				}
				if fainted {
					logEntries = append(logEntries, BattleLogEntry{
						Round:       bs.Turn,
						PokemonName: active.Name,
						Fainted:     true,
					})
				}
			} else {
				logEntries = append(logEntries, makeEntry(bs.Turn, wild, wildMove, 0, false, 1, false))
				if wc := applyEffectByID(bs, wild, active, wildMove, bs.Turn, &logEntries, sideFor(bs, active)); wc > 0 {
					setBattleWeather(bs, wc, &logEntries)
				}
				applyStatChanges(wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries, sideFor(bs, active))
				applyHeal(wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries)
				handleCustomStatusMove(bs, wild, active, wildMove, bs.Turn, &logEntries)
				applySideMove(bs, wild, active, wildMove, &logEntries)
				handlePhazeMove(bs, wild, active, wildMove, &logEntries)
			}
			// Дикий мог выгнать игрока Рыком/Вихрем — перечитываем активного.
			active = &bs.PlayerTeam[bs.ActiveIdx]
		}
	}

	// Конец хода: погода и таймеры стороны (как в SwitchHandler).
	postProcessWeather(bs, &logEntries)
	tickSideTimers(bs, &logEntries)

	// Обморок активного — как в MoveHandler: смена или поражение.
	respType := "catch_fail"
	if active.IsFainted {
		hasMore := false
		for _, p := range bs.PlayerTeam {
			if !p.IsFainted {
				hasMore = true
				break
			}
		}
		if hasMore {
			respType = "need_switch"
		} else {
			respType = "defeat"
			bs.Finished = true
			saveBattleState(bs)
			delete(battleSessions, req.BattleID)
		}
	}

	resp := BattleResponse{
		Type:         respType,
		Log:          logEntries,
		Player:       copyBattlePokemon(active),
		Wild:         copyBattlePokemon(wild),
		WeatherID:    bs.BattleWeatherID,
		WeatherName:  bs.BattleWeatherName,
		WeatherTurns: bs.WeatherTurns,
	}
	if respType == "need_switch" {
		team := make([]BattlePokemon, len(bs.PlayerTeam))
		for i := range bs.PlayerTeam {
			team[i] = *copyBattlePokemon(&bs.PlayerTeam[i])
		}
		resp.Team = team
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
