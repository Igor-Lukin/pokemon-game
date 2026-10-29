package main

// Временная утилита для живого теста: заменить команду ADMIN (user_id=2)
// на указанных покемонов 1 уровня с заданными характерами и ходами.
// Использует единую точку создания utils.CreateUserPokemon.

import (
	"fmt"
	"log"

	"pokemon-game/server/db"
	"pokemon-game/server/utils"
)

type teamSpec struct {
	SpeciesID int
	NatureID  int
	EggMoveID int // 0 = без дополнительного яйцевого хода
}

func main() {
	db.InitDB()
	defer db.CloseDB()

	// Освобождаем слоты 1..6 (старая команда уходит в питомник position=0).
	if _, err := db.DB.Exec("UPDATE user_pokemons SET position = 0 WHERE user_id = 2 AND position BETWEEN 1 AND 6"); err != nil {
		log.Fatal(err)
	}

	specs := []teamSpec{
		{SpeciesID: 120, NatureID: 22},              // Старью, Скромный (SpA+/Atk-)
		{SpeciesID: 909, NatureID: 7, EggMoveID: 519}, // Фуэкоко, Наглый (Def+/Atk-), Потягивание (Slack Off)
		{SpeciesID: 280, NatureID: 22},              // Ралтс, Скромный
		{SpeciesID: 440, NatureID: 5},               // Хаппини, Мирный (SpD+/Atk-)
		{SpeciesID: 227, NatureID: 15},              // Скармори, Озорной (Def+/SpA-)
		{SpeciesID: 532, NatureID: 12},              // Тимбёрр, Непреклонный (Atk+/SpA-)
	}

	for _, s := range specs {
		gender, err := db.RollGender(s.SpeciesID)
		if err != nil {
			gender = db.GenderNeutral
		}
		iv := [6]int{31, 31, 31, 31, 31, 31}
		id, err := utils.CreateUserPokemon(2, s.SpeciesID, 1, iv, s.NatureID, gender, 0, 0, false)
		if err != nil {
			log.Printf("create #%d: %v", s.SpeciesID, err)
			continue
		}
		teachMoves(id, s.SpeciesID, 1, s.EggMoveID)
		fmt.Printf("created user_pokemon=%d species=%d nature=%d\n", id, s.SpeciesID, s.NatureID)
	}
}

func teachMoves(userPokemonID, speciesID, level, extraMoveID int) {
	type mv struct {
		id, pp int
	}
	var pool []mv
	rows, err := db.DB.Query(`
		SELECT pa.attack_id, a.pp
		FROM pokemon_attacks pa
		JOIN attacks a ON a.id = pa.attack_id
		WHERE pa.pokemon_id = ? AND pa.learn_method_id = 1 AND pa.level_learned_at <= ?
		ORDER BY pa.level_learned_at, a.id`, speciesID, level)
	if err == nil {
		for rows.Next() {
			var m mv
			if rows.Scan(&m.id, &m.pp) == nil {
				pool = append(pool, m)
			}
		}
		rows.Close()
	}

	chosen := make([]mv, 0, 4)
	if extraMoveID > 0 {
		var pp int
		if db.DB.QueryRow("SELECT pp FROM attacks WHERE id = ?", extraMoveID).Scan(&pp) == nil {
			chosen = append(chosen, mv{id: extraMoveID, pp: pp})
		}
	}
	for _, m := range pool {
		if len(chosen) >= 4 {
			break
		}
		if m.id == extraMoveID {
			continue
		}
		chosen = append(chosen, m)
	}

	// Сброс активных слотов и выдача выбранных ходов.
	db.DB.Exec("UPDATE user_pokemon_learnable_attacks SET is_learned = 0, position = NULL WHERE user_pokemon_id = ?", userPokemonID)
	for i, m := range chosen {
		source := "level"
		if m.id == extraMoveID {
			source = "egg"
		}
		db.DB.Exec(`
			INSERT OR IGNORE INTO user_pokemon_learnable_attacks
				(user_pokemon_id, attack_id, level_available, is_learned, position, current_pp, source)
			VALUES (?, ?, 0, 0, NULL, ?, ?)`, userPokemonID, m.id, m.pp, source)
		db.DB.Exec(`
			UPDATE user_pokemon_learnable_attacks
			SET is_learned = 1, position = ?, current_pp = ?, source = ?
			WHERE user_pokemon_id = ? AND attack_id = ?`, i+1, m.pp, source, userPokemonID, m.id)
	}
}
