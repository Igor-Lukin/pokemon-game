package utils

import (
	"log"
	"pokemon-game/server/db"
)

// CreateUserPokemon — ЕДИНАЯ точка создания экземпляра покемона у игрока.
// Зовут и admin-выдача (GivePokemonHandler), и ловля (catch.go), и любое
// будущее разведение — чтобы роллы и расчёт статов не дублировались
// (мандат HANDOFF_gender_attractiveness).
//
// Пол разыгрывается ВНЕ этой функции (у дикого он уже разыгран в бою) и
// передаётся сюда параметром gender. Привлекательность роллится здесь
// (db.RollAttractiveness) — она ни от чего не зависит.
//
// Помещать в пакет db нельзя: расчёт статов (CalculateStats/GetNatureFactors)
// живёт в utils, а utils уже импортирует db — обратный импорт дал бы цикл.
// Поэтому единая точка — здесь, в utils.
//
// Position: первый свободный слот 1..6 (КОМАНДА); если все заняты — 0 (ПИТОМНИК).
//
// Пост-эффекты болов (фаза 2):
//   geneBonus      — прибавка ко ВСЕМ генам (IV), кап 0..31 (Даркбол/Супердаркбол);
//   happinessBonus — стартовое счастье пойманного (база 0);
//   forceNursery   — принудительно в питомник (Трансбол), минуя слоты команды.
// Admin-выдача зовёт с (0, 0, false) — обычное создание.
func CreateUserPokemon(userID, pokemonID, level int, iv [6]int, natureID int, gender string,
	geneBonus, happinessBonus int, forceNursery bool) (int, error) {

	// Бонус к генам (IV), кап 0..31.
	if geneBonus != 0 {
		for i := range iv {
			v := iv[i] + geneBonus
			if v > 31 {
				v = 31
			}
			if v < 0 {
				v = 0
			}
			iv[i] = v
		}
	}
	// Базовые статы вида. Порядок: HP,Atk,Def,Spd,SpAtk,SpDef.
	var base [6]int
	err := db.DB.QueryRow(`
		SELECT base_hp, base_attack, base_defense, base_speed,
		       base_special_attack, base_special_defense
		FROM pokemons WHERE id = ?`, pokemonID).Scan(
		&base[0], &base[1], &base[2], &base[3], &base[4], &base[5])
	if err != nil {
		return 0, err
	}

	natureFactors, err := GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0}
	}
	ev := [6]int{0, 0, 0, 0, 0, 0}
	train := [6]float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0}

	currentHP, maxHP, attack, defense, speed, specialAttack, specialDefense :=
		CalculateStats(base, iv, ev, level, natureFactors, train)

	attractiveness := db.RollAttractiveness()

	// Position: питомник (0) принудительно (Трансбол) ИЛИ первый свободный
	// слот команды 1..6, иначе питомник (0).
	position := 0
	if !forceNursery {
		occupied := map[int]bool{}
		rows, err := db.DB.Query(
			"SELECT position FROM user_pokemons WHERE user_id = ? AND position BETWEEN 1 AND 6", userID)
		if err == nil {
			for rows.Next() {
				var p int
				if rows.Scan(&p) == nil {
					occupied[p] = true
				}
			}
			rows.Close()
		}
		for slot := 1; slot <= 6; slot++ {
			if !occupied[slot] {
				position = slot
				break
			}
		}
	}

	result, err := db.DB.Exec(`
		INSERT INTO user_pokemons (
			user_id, pokemon_id, level, experience,
			current_hp, max_hp, attack, defense, speed,
			special_attack, special_defense,
			iv_hp, iv_attack, iv_defense, iv_speed,
			iv_special_attack, iv_special_defense,
			ev_hp, ev_attack, ev_defense, ev_speed,
			ev_special_attack, ev_special_defense,
			available_ev, position, status_id, nature_id,
			gender, attractiveness, happiness
		) VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 1, ?, ?, ?, ?)`,
		userID, pokemonID, level, currentHP, maxHP, attack, defense, speed, specialAttack, specialDefense,
		iv[0], iv[1], iv[2], iv[3], iv[4], iv[5],
		ev[0], ev[1], ev[2], ev[3], ev[4], ev[5],
		position, natureID, gender, attractiveness, happinessBonus)
	if err != nil {
		return 0, err
	}
	userPokemonID, _ := result.LastInsertId()

	// Способность экземпляра: СЛУЧАЙНАЯ базовая вида (скрытая при обычном создании
	// не выпадает). Единая точка => ловля/выдача/вылупление получают её автоматически.
	// Нет базовых у вида -> NULL (покемон без способности).
	if err := db.AssignBasicAbility(int(userPokemonID), pokemonID); err != nil {
		log.Printf("CreateUserPokemon: AssignBasicAbility error: %v", err)
	}

	if err := db.InitLearnableAttacks(int(userPokemonID), pokemonID, level); err != nil {
		log.Printf("CreateUserPokemon: InitLearnableAttacks error: %v", err)
	}
	return int(userPokemonID), nil
}
