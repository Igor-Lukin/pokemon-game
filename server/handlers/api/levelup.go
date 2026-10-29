package api

import (
	"log"
	"pokemon-game/server/db"
	"pokemon-game/server/utils"
)

// applyLevelUp handles the common level-up logic: evolution check, stat recalculation,
// DB update, and learnable attack re-init. Returns the new pokemon ID (may have evolved),
// evolution info, new max_hp (for battle state sync), and an error.
func applyLevelUp(userPokemonID, pokemonID, level int, iv, ev [6]int, natureID int, availableEV int) (int, *EvolutionInfo, int, error) {
	evoInfo, err := checkEvolutionFor(userPokemonID, pokemonID, level)
	if err != nil {
		return 0, nil, 0, err
	}

	newPokemonID := pokemonID
	if evoInfo != nil && evoInfo.CanEvolve {
		newPokemonID = evoInfo.EvolvesToID
	}

	var base [6]int
	err = db.DB.QueryRow(`
		SELECT base_hp, base_attack, base_defense, base_speed,
		       base_special_attack, base_special_defense
		FROM pokemons WHERE id = ?
	`, newPokemonID).Scan(&base[0], &base[1], &base[2], &base[3], &base[4], &base[5])
	if err != nil {
		return 0, nil, 0, err
	}

	natureFactors, err := utils.GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1, 1, 1, 1, 1, 1}
	}
	train := [6]float64{1, 1, 1, 1, 1, 1}

	_, maxHP, atk, def, spd, spAtk, spDef :=
		utils.CalculateStats(base, iv, ev, level, natureFactors, train)

	_, err = db.DB.Exec(`
		UPDATE user_pokemons SET
			level = ?, pokemon_id = ?,
			max_hp = ?, attack = ?, defense = ?,
			speed = ?, special_attack = ?, special_defense = ?,
			current_hp = max_hp, available_ev = ?
		WHERE id = ?
	`, level, newPokemonID, maxHP, atk, def, spd, spAtk, spDef, availableEV, userPokemonID)
	if err != nil {
		return 0, nil, 0, err
	}

	if err := db.InitLearnableAttacks(userPokemonID, newPokemonID, level); err != nil {
		log.Printf("applyLevelUp: InitLearnableAttacks error: %v", err)
	}

	// [ability] ВТОРАЯ точка эволюции (авто-эволюция при левелапе после боя / от
	// Леденца). Старый вид = параметр pokemonID (до Exec). Ремап только при реальной
	// смене вида. Единая точка ремапа — db.RemapAbilityOnEvolve (см. §5.4 ТЗ).
	if newPokemonID != pokemonID {
		if err := db.RemapAbilityOnEvolve(userPokemonID, pokemonID, newPokemonID); err != nil {
			log.Printf("applyLevelUp: RemapAbilityOnEvolve error: %v", err)
		}
	}

	return newPokemonID, evoInfo, maxHP, nil
}
