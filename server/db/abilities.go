package db

import (
	"database/sql"
	"log"
	mathrand "math/rand"
)

// abilities.go — ЕДИНАЯ точка правды по способностям (абилкам) экземпляра.
//
// Модель (дизайн владельца, HANDOFF_abilities_framework):
//   - У ВИДА (pokemon_abilities) 1..3 слота: 1 = базовая X (есть всегда),
//     2 = базовая Y (опц.), 3 = СКРЫТАЯ (опц.). Согласованность: slot=3 <=> is_hidden=1.
//   - У ЭКЗЕМПЛЯРА (user_pokemons.ability_id/ability_slot) РОВНО ОДНА способность,
//     закреплённая при создании (случайная БАЗОВАЯ). Скрытая при обычном создании
//     НЕ выпадает.
//   - При эволюции слот СОХРАНЯЕТСЯ (см. RemapAbilityOnEvolve).
//
// ВАЖНО: любая точка создания экземпляра обязана назначать способность через
// RollBasicAbility (единая точка — utils.CreateUserPokemon), а ремап при эволюции —
// только через RemapAbilityOnEvolve (зовут EvolveHandler и applyLevelUp).

// AbilitySlot — способность вида с её слотом.
type AbilitySlot struct {
	AbilityID int
	Slot      int
}

// SpeciesBasicAbilities — базовые способности вида (slot 1/2, is_hidden=0),
// отсортированные по slot. Пусто, если у вида нет базовых.
func SpeciesBasicAbilities(pokemonID int) []AbilitySlot {
	rows, err := DB.Query(`
		SELECT ability_id, COALESCE(slot, 0) AS slot
		FROM pokemon_abilities
		WHERE pokemon_id = ? AND COALESCE(is_hidden, 0) = 0
		ORDER BY slot, ability_id`, pokemonID)
	if err != nil {
		log.Printf("SpeciesBasicAbilities: query error for pokemon %d: %v", pokemonID, err)
		return nil
	}
	defer rows.Close()
	var res []AbilitySlot
	for rows.Next() {
		var a AbilitySlot
		if rows.Scan(&a.AbilityID, &a.Slot) == nil {
			res = append(res, a)
		}
	}
	return res
}

// SpeciesHiddenAbility — скрытая способность вида (slot=3), если есть.
func SpeciesHiddenAbility(pokemonID int) (abilityID int, ok bool) {
	err := DB.QueryRow(`
		SELECT ability_id FROM pokemon_abilities
		WHERE pokemon_id = ? AND (COALESCE(is_hidden, 0) = 1 OR slot = 3)
		ORDER BY ability_id LIMIT 1`, pokemonID).Scan(&abilityID)
	if err != nil {
		return 0, false
	}
	return abilityID, true
}

// RollBasicAbility — СЛУЧАЙНАЯ базовая способность вида (для создания экземпляра
// и для транзиентной выдачи диким/тренерским бойцам). ok=false, если у вида нет
// базовых (покемон без способности).
func RollBasicAbility(pokemonID int) (abilityID int, slot int, ok bool) {
	basics := SpeciesBasicAbilities(pokemonID)
	if len(basics) == 0 {
		return 0, 0, false
	}
	pick := basics[mathrand.Intn(len(basics))]
	return pick.AbilityID, pick.Slot, true
}

// AbilityEffectKey — машиночитаемый ключ эффекта по id способности (для боя).
// Пустая строка, если не задан или способность не найдена.
func AbilityEffectKey(abilityID int) string {
	var key sql.NullString
	err := DB.QueryRow("SELECT effect_key FROM abilities WHERE id = ?", abilityID).Scan(&key)
	if err != nil {
		return ""
	}
	return key.String
}

// AbilityNameKey — имя и effect_key способности по id (для загрузки в бой).
// Пустые строки, если не найдена.
func AbilityNameKey(abilityID int) (name, key string) {
	var n, k sql.NullString
	DB.QueryRow("SELECT name, effect_key FROM abilities WHERE id = ?", abilityID).Scan(&n, &k)
	return n.String, k.String
}

// setInstanceAbility — записать способность экземпляра. has=false => NULL/NULL.
func setInstanceAbility(userPokemonID, abilityID, slot int, has bool) error {
	if !has {
		_, err := DB.Exec(
			"UPDATE user_pokemons SET ability_id = NULL, ability_slot = NULL WHERE id = ?",
			userPokemonID)
		return err
	}
	_, err := DB.Exec(
		"UPDATE user_pokemons SET ability_id = ?, ability_slot = ? WHERE id = ?",
		abilityID, slot, userPokemonID)
	return err
}

// AssignBasicAbility — назначить экземпляру СЛУЧАЙНУЮ базовую способность его вида.
// Нет базовых -> NULL/NULL. Вызывается из единой точки создания (utils.CreateUserPokemon).
func AssignBasicAbility(userPokemonID, pokemonID int) error {
	abilityID, slot, ok := RollBasicAbility(pokemonID)
	return setInstanceAbility(userPokemonID, abilityID, slot, ok)
}

// RemapAbilityOnEvolve — пересчитать способность экземпляра при эволюции по правилу
// СОХРАНЕНИЯ СЛОТА. ЕДИНАЯ точка ремапа: зовут EvolveHandler (ручная эволюция) и
// applyLevelUp (авто-эволюция после боя / от Леденца).
//
// ВАЖНО: oldPokemonID — вид ДО эволюции (прочитать до UPDATE pokemon_id),
// newPokemonID — вид ПОСЛЕ. Правило (владелец):
//   - скрытая (slot 3): -> скрытая новой формы; нет скрытой -> случайная базовая;
//   - базовая (slot 1/2):
//       * число базовых пре == пост  -> сохранить ТОТ ЖЕ слот (фолбэк первая базовая);
//       * различается                 -> СЛУЧАЙНАЯ из базовых новой формы;
//   - у экземпляра не было способности (NULL) -> случайная базовая новой формы;
//   - у новой формы нет базовых -> NULL (без способности).
func RemapAbilityOnEvolve(userPokemonID, oldPokemonID, newPokemonID int) error {
	var curSlot sql.NullInt64
	err := DB.QueryRow("SELECT ability_slot FROM user_pokemons WHERE id = ?", userPokemonID).Scan(&curSlot)
	if err != nil {
		return err
	}

	// Не было способности -> дать случайную базовую новой формы (или NULL).
	if !curSlot.Valid {
		return AssignBasicAbility(userPokemonID, newPokemonID)
	}
	slot := int(curSlot.Int64)

	// Скрытая -> скрытая новой формы; иначе случайная базовая.
	if slot == 3 {
		if hid, ok := SpeciesHiddenAbility(newPokemonID); ok {
			return setInstanceAbility(userPokemonID, hid, 3, true)
		}
		log.Printf("RemapAbilityOnEvolve: up=%d новая форма %d без скрытой -> случайная базовая",
			userPokemonID, newPokemonID)
		return AssignBasicAbility(userPokemonID, newPokemonID)
	}

	// Базовая (slot 1/2).
	postBasics := SpeciesBasicAbilities(newPokemonID)
	if len(postBasics) == 0 {
		log.Printf("RemapAbilityOnEvolve: up=%d новая форма %d без базовых -> без способности",
			userPokemonID, newPokemonID)
		return setInstanceAbility(userPokemonID, 0, 0, false)
	}
	preBasics := SpeciesBasicAbilities(oldPokemonID)

	if len(preBasics) == len(postBasics) {
		// Равное число базовых -> сохранить тот же слот.
		for _, b := range postBasics {
			if b.Slot == slot {
				return setInstanceAbility(userPokemonID, b.AbilityID, b.Slot, true)
			}
		}
		// Слота с таким номером нет -> фолбэк на первую базовую.
		return setInstanceAbility(userPokemonID, postBasics[0].AbilityID, postBasics[0].Slot, true)
	}

	// Число базовых различается -> случайная из базовых новой формы.
	pick := postBasics[mathrand.Intn(len(postBasics))]
	return setInstanceAbility(userPokemonID, pick.AbilityID, pick.Slot, true)
}

// --- Миграции данных (зовутся из database.go, идемпотентны) ---

// migrateAbilitySlots — проставить slot существующим строкам pokemon_abilities,
// у которых slot ещё не задан (NULL):
//   - is_hidden=1        -> slot=3;
//   - is_hidden=0        -> slot 1,2,… по порядку ability_id внутри вида.
// Идемпотентно: трогает только строки с slot IS NULL.
func migrateAbilitySlots() {
	// Скрытые -> 3.
	DB.Exec("UPDATE pokemon_abilities SET slot = 3 WHERE slot IS NULL AND COALESCE(is_hidden, 0) = 1")

	// Базовые -> 1,2,… по порядку внутри вида. Сначала собираем (rows закрыть до UPDATE).
	rows, err := DB.Query(`
		SELECT pokemon_id, ability_id FROM pokemon_abilities
		WHERE slot IS NULL AND COALESCE(is_hidden, 0) = 0
		ORDER BY pokemon_id, ability_id`)
	if err != nil {
		log.Printf("migrateAbilitySlots: query error: %v", err)
		return
	}
	type row struct{ pokemonID, abilityID int }
	var pending []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.pokemonID, &r.abilityID) == nil {
			pending = append(pending, r)
		}
	}
	rows.Close()

	curSpecies, slot := -1, 0
	for _, r := range pending {
		if r.pokemonID != curSpecies {
			curSpecies = r.pokemonID
			slot = 0
		}
		slot++
		if _, err := DB.Exec(
			"UPDATE pokemon_abilities SET slot = ? WHERE pokemon_id = ? AND ability_id = ?",
			slot, r.pokemonID, r.abilityID); err != nil {
			log.Printf("migrateAbilitySlots: update error species=%d ability=%d: %v",
				r.pokemonID, r.abilityID, err)
		}
	}
}

// backfillInstanceAbilities — назначить СЛУЧАЙНУЮ базовую способность экземплярам,
// у которых ability_id ещё не задан и у чьего вида есть базовые. Идемпотентно:
// трогает только ability_id IS NULL.
func backfillInstanceAbilities() {
	rows, err := DB.Query("SELECT id, pokemon_id FROM user_pokemons WHERE ability_id IS NULL")
	if err != nil {
		log.Printf("backfillInstanceAbilities: query error: %v", err)
		return
	}
	type row struct{ upID, pokemonID int }
	var pending []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.upID, &r.pokemonID) == nil {
			pending = append(pending, r)
		}
	}
	rows.Close()

	n := 0
	for _, r := range pending {
		abilityID, slot, ok := RollBasicAbility(r.pokemonID)
		if !ok {
			continue // вид без базовых — оставить NULL
		}
		if err := setInstanceAbility(r.upID, abilityID, slot, true); err != nil {
			log.Printf("backfillInstanceAbilities: set error up=%d: %v", r.upID, err)
			continue
		}
		n++
	}
	if n > 0 {
		log.Printf("backfillInstanceAbilities: назначено способностей экземплярам: %d", n)
	}
}
