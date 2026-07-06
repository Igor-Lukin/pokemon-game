package db

import "log"

func InitLearnableAttacks(userPokemonID, pokemonID, level int) error {
	rows, err := DB.Query(`
		SELECT pa.attack_id, pa.level_learned_at
		FROM pokemon_attacks pa
		WHERE pa.pokemon_id = ? AND pa.learn_method_id = 1 AND pa.level_learned_at <= ?
	`, pokemonID, level)
	if err != nil {
		return err
	}
	defer rows.Close()

	type attackEntry struct {
		attackID      int
		levelAvailable int
	}
	var attacks []attackEntry
	for rows.Next() {
		var a attackEntry
		if err := rows.Scan(&a.attackID, &a.levelAvailable); err != nil {
			log.Printf("InitLearnableAttacks scan error: %v", err)
			continue
		}
		attacks = append(attacks, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	var inserted int
	for _, a := range attacks {
		if _, err := DB.Exec(`
			INSERT OR IGNORE INTO user_pokemon_learnable_attacks
				(user_pokemon_id, attack_id, level_available)
			VALUES (?, ?, ?)
		`, userPokemonID, a.attackID, a.levelAvailable); err != nil {
			log.Printf("InitLearnableAttacks insert error: %v", err)
		} else {
			inserted++
		}
		// Set current_pp for newly inserted rows (and update existing if still 0)
		DB.Exec(`
			UPDATE user_pokemon_learnable_attacks
			SET current_pp = (SELECT pp FROM attacks WHERE attacks.id = ?)
			WHERE user_pokemon_id = ? AND attack_id = ? AND current_pp = 0
		`, a.attackID, userPokemonID, a.attackID)
	}
	log.Printf("InitLearnableAttacks: user_pokemon=%d pokemon=%d level=%d — inserted %d attacks", userPokemonID, pokemonID, level, inserted)
	return nil
}
