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
//   - число базовых пре == пост  -> сохранить ТОТ ЖЕ слот (фолбэк первая базовая);
//   - различается                 -> СЛУЧАЙНАЯ из базовых новой формы;
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

// migrateAbilityBatch5Keys — проставить effect_key сложным способностям батча 5.
// Идемпотентно: трогает только строки с пустым/NULL ключом, не перетирая ручные
// правки. Левитация в каталоге league17 есть (id 91) — реализуем её, а не аналог.
func migrateAbilityBatch5Keys() {
	keys := map[int]string{
		20:  "snow_cloak",   // Белотелый
		91:  "levitate",     // Левитация
		136: "sand_veil",    // Пескоструй
		173: "shields_down", // Сброс брони
		183: "solar_power",  // Солнечная батарея
		186: "schooling",    // Стайность
		189: "dry_skin",     // Сухая кожа
		197: "poison_heal",  // Токсифаг
		200: "totem",        // Тотем
		215: "weak_armor",   // Хрупкая броня
	}
	for id, key := range keys {
		DB.Exec("UPDATE abilities SET effect_key = ? WHERE id = ? AND COALESCE(effect_key, '') = ''", key, id)
	}
}

// migrateAbilityBatch6Keys — effect_key для топ-20 способностей по числу носителей
// (батч 6). Идемпотентно: трогает только незаполненные ключи.
// «Обжорство» (122) намеренно не размечено: в движке пока нет расхода лечащих
// предметов в бою, эффект будет добавлен вместе с этой механикой.
func migrateAbilityBatch6Keys() {
	keys := map[int]string{
		3:   "chlorophyll",  // Хлорофилл
		24:  "inner_focus",  // Бесстрашие
		29:  "guts",         // Болевой порог
		31:  "pressure",     // Величие
		33:  "own_tempo",    // Внутренний ритм
		36:  "swift_swim",   // Водоплавающий
		40:  "hydration",    // Впитывание
		49:  "sheer_force",  // Грубая сила
		63:  "oblivious",    // Забывчивость
		73:  "sturdy",       // Здоровяк
		95:  "leaf_guard",   // Лиственный щит
		101: "technician",   // Мастерство
		119: "hustle",       // Нетерпеливость
		141: "mold_breaker", // Подавление способностей
		148: "infiltrator",  // Преодоление барьеров
		166: "regenerator",  // Регенератор
		191: "damp",         // Сырость
		194: "telepathy",    // Телепатия
		206: "overcoat",     // Укрытие
		230: "frisk",        // Проницательность
	}
	for id, key := range keys {
		DB.Exec("UPDATE abilities SET effect_key = ? WHERE id = ? AND COALESCE(effect_key, '') = ''", key, id)
	}
}

// migrateAbilityBatch7Keys — effect_key для расширенного батча 7+ (все
// способности, у которых появилась реальная логика в abilities_extra.go).
// Идемпотентно: трогает только незаполненные ключи.
func migrateAbilityBatch7Keys() {
	keys := map[int]string{
		1:   "lightning_rod",
		6:   "tinted_lens",
		14:  "synchronize",
		15:  "water_defense",
		17:  "flare_boost",
		22:  "berserk",
		25:  "aroma_veil",
		35:  "water_bubble",
		37:  "fairy_aura",
		38:  "flower_gift",
		39:  "pixilate",
		41:  "stamina",
		45:  "liquid_ooze",
		50:  "aftermath",
		51:  "defiant",
		52:  "queenly_majesty",
		53:  "long_reach",
		54:  "simple",
		57:  "friend_guard",
		58:  "early_bird",
		59:  "grass_pelt",
		60:  "iron_fist",
		61:  "liquid_voice",
		64:  "download",
		65:  "moxie",
		66:  "steelworker",
		67:  "ambush",
		68:  "stall",
		70:  "victory_star",
		71:  "beast_boost",
		72:  "soundproof",
		74:  "aerilate",
		75:  "stench",
		77:  "serene_grace",
		78:  "illusion",
		79:  "anticipation",
		80:  "ion_crown",
		82:  "rock_head",
		83:  "pickpocket",
		84:  "coma",
		85:  "competitive",
		87:  "corrosion",
		88:  "bad_dreams",
		90:  "magma_armor",
		94:  "sticky_hold",
		96:  "magic_bounce",
		97:  "magic_guard",
		98:  "magnet_pull",
		99:  "disguise",
		100: "skill_link",
		102: "moody",
		103: "misty_surge",
		105: "meteor_armor",
		107: "cloud_nine",
		108: "mercy",
		109: "protean",
		110: "water_veil",
		112: "refrigerate",
		115: "strong_jaw",
		116: "multitype",
		117: "mummy",
		118: "unaware",
		120: "slow_start",
		123: "heatproof",
		124: "prankster",
		125: "effect_spore",
		126: "aftermath_quarter",
		127: "defeatist",
		128: "plus",
		129: "mega_launcher",
		130: "sweet_veil",
		131: "triage",
		132: "primordial_armor",
		133: "mold_breaker",
		134: "mold_breaker",
		135: "marvel_scale",
		137: "sand_force",
		140: "aura_break",
		142: "plus",
		144: "anger_point",
		145: "justified",
		146: "forewarn",
		151: "natural_cure",
		152: "suction_cups",
		153: "adaptability",
		154: "arena_trap",
		155: "queenly_majesty",
		156: "cursed_body",
		157: "truant",
		158: "normalize",
		159: "contrary",
		161: "bulletproof",
		162: "tangled_feet",
		164: "unburden",
		165: "klutz",
		167: "toxic_boost",
		169: "steadfast",
		171: "imposter",
		172: "analytic",
		174: "scrappy",
		175: "compassion",
		177: "type_item",
		178: "slush_rush",
		184: "rivalry",
		185: "no_guard",
		190: "super_luck",
		192: "dark_aura",
		196: "shadow_tag",
		198: "poison_touch",
		199: "light_metal",
		202: "grassy_surge",
		203: "misty_surge",
		204: "heavy_metal",
		207: "hyper_cutter",
		208: "gale_wings",
		210: "rattled",
		211: "magician",
		213: "sand_stream",
		214: "color_change",
		216: "flower_veil",
		217: "healer",
		218: "shield_dust",
		221: "sand_rush",
		222: "cloud_nine",
		224: "quick_feet",
		225: "electric_surge",
		226: "trace",
		227: "wonder_guard",
		232: "multiscale",
		234: "wind_rider",
		235: "wonder_skin",
		236: "flare_boost",
		237: "sharpness",
		238: "zen_mode",
		240: "galvanize",
		241: "surge_surfer",
		242: "battle_bond",
		244: "tangled_hair",
		246: "psychic_surge",
		247: "full_metal_body",
		248: "neuroforce",
		249: "defiant",
		250: "sand_rush",
		252: "chilling_neigh",
		253: "cotton_down",
		254: "dauntless_shield",
		255: "dragon_maw",
		256: "grim_neigh",
		259: "ice_face",
		260: "ice_scales",
		261: "intrepid_sword",
		262: "libero",
		263: "light_metal",
		264: "mirror_armor",
		265: "perish_body",
		266: "power_spot",
		267: "suction_cups",
		268: "punk_rock",
		270: "sand_spit",
		271: "screen_cleaner",
		272: "suction_cups",
		273: "steam_engine",
		274: "steely_spirit",
		275: "transistor",
		276: "unseen_fist",
		277: "wandering_spirit",
		278: "sticky_aroma",
		279: "seed_sower",
		280: "thermal_exchange",
		281: "rage_shell",
		282: "purifying_salt",
		283: "well_baked_body",
		284: "wind_rider",
		285: "guard_dog",
		286: "rocky_payload",
		287: "wind_power",
		288: "zero_to_hero",
		290: "electromorphosis",
		291: "protosynthesis",
		292: "quark_drive",
		293: "good_as_gold",
		294: "vessel_of_ruin",
		295: "sword_of_ruin",
		296: "tablets_of_ruin",
		297: "beads_of_ruin",
		298: "orichalcum_pulse",
		299: "hadron_engine",
		300: "opportunist",
		302: "supreme_overlord",
		304: "toxic_debris",
		305: "armor_tail",
		306: "earth_eater",
		307: "mycelium_might",
		308: "sweet_syrup",
		310: "toxic_chain",
		312: "poison_puppeteer",
	}
	for id, key := range keys {
		DB.Exec("UPDATE abilities SET effect_key = ? WHERE id = ? AND COALESCE(effect_key, '') = ''", key, id)
	}
}

// migrateAbilitySlots — проставить slot существующим строкам pokemon_abilities,
// у которых slot ещё не задан (NULL):
//   - is_hidden=1        -> slot=3;
//   - is_hidden=0        -> slot 1,2,… по порядку ability_id внутри вида.
//
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
