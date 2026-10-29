package api

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	"pokemon-game/server/db"
)

// TestEvolveWithItemAllLines проверяет все 49 линий предметной эволюции
// (10 камней + Oval Stone + 4 trade-предмета) на изолированной временной БД:
// эволюция по предмету, списание ровно одного предмета, no-op на
// неподходящий предмет и отсутствие эволюции без предмета.
func TestEvolveWithItemAllLines(t *testing.T) {
	oldDB := db.DB
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "evo_items.db"))
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	conn.SetMaxOpenConns(1)
	db.DB = conn
	defer func() {
		db.DB = oldDB
		conn.Close()
	}()

	schema := []string{
		`CREATE TABLE pokemons (
			id INTEGER PRIMARY KEY, name TEXT, image_url TEXT,
			base_hp INTEGER, base_attack INTEGER, base_defense INTEGER,
			base_speed INTEGER, base_special_attack INTEGER, base_special_defense INTEGER,
			evolves_from_id INTEGER, evolution_method_id INTEGER,
			evolution_level INTEGER, evolution_item_id INTEGER)`,
		`CREATE TABLE user_pokemons (
			id INTEGER PRIMARY KEY, user_id INTEGER, pokemon_id INTEGER,
			level INTEGER, nature_id INTEGER, current_hp INTEGER, max_hp INTEGER,
			attack INTEGER, defense INTEGER, speed INTEGER,
			special_attack INTEGER, special_defense INTEGER,
			iv_hp INTEGER, iv_attack INTEGER, iv_defense INTEGER, iv_speed INTEGER,
			iv_special_attack INTEGER, iv_special_defense INTEGER,
			ev_hp INTEGER, ev_attack INTEGER, ev_defense INTEGER, ev_speed INTEGER,
			ev_special_attack INTEGER, ev_special_defense INTEGER,
			ability_id INTEGER, ability_slot INTEGER)`,
		`CREATE TABLE user_items (
			user_id INTEGER, item_id INTEGER, quantity INTEGER,
			PRIMARY KEY(user_id, item_id))`,
		`CREATE TABLE natures (
			id INTEGER PRIMARY KEY, increased_stat TEXT, decreased_stat TEXT)`,
		`CREATE TABLE pokemon_attacks (
			pokemon_id INTEGER, attack_id INTEGER,
			learn_method_id INTEGER, level_learned_at INTEGER)`,
		`CREATE TABLE pokemon_abilities (
			pokemon_id INTEGER, ability_id INTEGER, is_hidden BOOLEAN, slot INTEGER)`,
	}
	for _, q := range schema {
		if _, err := conn.Exec(q); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}

	// Старый вид, новый вид, предмет, метод.
	lines := []struct {
		oldID, newID, itemID, methodID int
	}{
		// Sun Stone (151).
		{44, 182, 151, 3}, {191, 192, 151, 3}, {546, 547, 151, 3}, {548, 549, 151, 3}, {694, 695, 151, 3},
		// Trade-предметы gen2 (152-155).
		{61, 186, 152, 2}, {79, 199, 152, 2},
		{95, 208, 153, 2}, {123, 212, 153, 2},
		{117, 230, 154, 2},
		{137, 233, 155, 2},
		// Fire Stone (156).
		{37, 38, 156, 3}, {58, 59, 156, 3}, {133, 136, 156, 3}, {513, 514, 156, 3}, {951, 952, 156, 3},
		// Water Stone (157).
		{61, 62, 157, 3}, {90, 91, 157, 3}, {120, 121, 157, 3}, {133, 134, 157, 3}, {271, 272, 157, 3}, {515, 516, 157, 3},
		// Thunder Stone (158).
		{25, 26, 158, 3}, {133, 135, 158, 3}, {82, 462, 158, 3}, {299, 476, 158, 3}, {603, 604, 158, 3}, {737, 738, 158, 3}, {938, 939, 158, 3},
		// Leaf Stone (159).
		{44, 45, 159, 3}, {70, 71, 159, 3}, {102, 103, 159, 3}, {274, 275, 159, 3}, {133, 470, 159, 3}, {511, 512, 159, 3},
		// Moon Stone (160).
		{30, 31, 160, 3}, {33, 34, 160, 3}, {35, 36, 160, 3}, {39, 40, 160, 3}, {300, 301, 160, 3}, {517, 518, 160, 3},
		// Shiny Stone (161).
		{315, 407, 161, 3}, {176, 468, 161, 3}, {572, 573, 161, 3}, {670, 671, 161, 3},
		// Dusk Stone (162).
		{200, 429, 162, 3}, {198, 430, 162, 3}, {608, 609, 162, 3}, {680, 681, 162, 3},
		// Dawn Stone (163).
		{281, 475, 163, 3}, {361, 478, 163, 3},
		// Ice Stone (164).
		{133, 471, 164, 3}, {739, 740, 164, 3}, {974, 975, 164, 3},
		// Oval Stone (165).
		{440, 113, 165, 3},
	}

	species := map[int]bool{}
	itemIDs := map[int]bool{}
	for _, l := range lines {
		species[l.oldID] = true
		species[l.newID] = true
		itemIDs[l.itemID] = true
	}
	for id := range species {
		if _, err := conn.Exec(`
			INSERT INTO pokemons
				(id, name, image_url, base_hp, base_attack, base_defense,
				 base_speed, base_special_attack, base_special_defense)
			VALUES (?, ?, '', 50, 50, 50, 50, 50, 50)
		`, id, "species-"+strconv.Itoa(id)); err != nil {
			t.Fatalf("insert species %d: %v", id, err)
		}
	}
	for i, l := range lines {
		if _, err := conn.Exec(`
			UPDATE pokemons
			SET evolves_from_id = ?, evolution_method_id = ?, evolution_item_id = ?
			WHERE id = ?
		`, l.oldID, l.methodID, l.itemID, l.newID); err != nil {
			t.Fatalf("link species %d: %v", l.newID, err)
		}
		if _, err := conn.Exec(`
			INSERT INTO user_pokemons
				(id, user_id, pokemon_id, level, nature_id, current_hp, max_hp,
				 attack, defense, speed, special_attack, special_defense,
				 iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
				 ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense)
			VALUES (?, 7, ?, 50, 0, 100, 100, 50, 50, 50, 50, 50,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		`, i+1, l.oldID); err != nil {
			t.Fatalf("insert user pokemon %d: %v", i+1, err)
		}
	}
	for itemID := range itemIDs {
		if _, err := conn.Exec(
			"INSERT INTO user_items (user_id, item_id, quantity) VALUES (7, ?, 2)",
			itemID,
		); err != nil {
			t.Fatalf("insert item %d: %v", itemID, err)
		}
	}

	for i, l := range lines {
		userPokemonID := i + 1
		// Общий стек на несколько линий (Sun Stone/King's Rock/Metal Coat),
		// поэтому перед каждой линией выставляем ровно 2 предмета.
		if _, err := conn.Exec(`
			INSERT INTO user_items (user_id, item_id, quantity) VALUES (7, ?, 2)
			ON CONFLICT(user_id, item_id) DO UPDATE SET quantity = 2
		`, l.itemID); err != nil {
			t.Fatalf("reset item %d: %v", l.itemID, err)
		}

		applied, msg, err := EvolveWithItem(7, userPokemonID, l.itemID)
		if err != nil {
			t.Fatalf("line %d->%d: %v", l.oldID, l.newID, err)
		}
		if !applied {
			t.Fatalf("line %d->%d not applied: %s", l.oldID, l.newID, msg)
		}

		var pokemonID, level, currentHP, maxHP int
		if err := conn.QueryRow(`
			SELECT pokemon_id, level, current_hp, max_hp
			FROM user_pokemons WHERE id = ?
		`, userPokemonID).Scan(&pokemonID, &level, &currentHP, &maxHP); err != nil {
			t.Fatalf("read evolved line %d: %v", l.oldID, err)
		}
		if pokemonID != l.newID {
			t.Fatalf("line %d: got species %d, want %d", l.oldID, pokemonID, l.newID)
		}
		if level != 50 {
			t.Fatalf("line %d: level changed to %d", l.oldID, level)
		}
		if currentHP != maxHP || maxHP <= 0 {
			t.Fatalf("line %d: hp not full after evolution (%d/%d)", l.oldID, currentHP, maxHP)
		}

		var qty int
		if err := conn.QueryRow(
			"SELECT quantity FROM user_items WHERE user_id = 7 AND item_id = ?",
			l.itemID,
		).Scan(&qty); err != nil {
			t.Fatalf("read item %d: %v", l.itemID, err)
		}
		if qty != 1 {
			t.Fatalf("line %d: item %d qty=%d, want 1", l.oldID, l.itemID, qty)
		}

		// Повторное применение того же предмета к уже эволюционировавшему виду — no-op.
		applied, msg, err = EvolveWithItem(7, userPokemonID, l.itemID)
		if err != nil {
			t.Fatalf("repeat line %d: %v", l.oldID, err)
		}
		if applied || msg == "" {
			t.Fatalf("repeat line %d should be no-op with message", l.oldID)
		}
		if err := conn.QueryRow(
			"SELECT quantity FROM user_items WHERE user_id = 7 AND item_id = ?",
			l.itemID,
		).Scan(&qty); err != nil {
			t.Fatalf("read repeat item %d: %v", l.itemID, err)
		}
		if qty != 1 {
			t.Fatalf("repeat line %d consumed extra item: qty=%d", l.oldID, qty)
		}
	}

	// Неподходящий предмет: King's Rock на Глума -> no-op, предмет не тратится.
	if _, err := conn.Exec(`
		INSERT INTO user_pokemons
			(id, user_id, pokemon_id, level, nature_id, current_hp, max_hp,
			 attack, defense, speed, special_attack, special_defense,
			 iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
			 ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense)
		VALUES (1001, 7, 44, 50, 0, 100, 100, 50, 50, 50, 50, 50,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	`); err != nil {
		t.Fatalf("insert wrong-item pokemon: %v", err)
	}
	applied, msg, err := EvolveWithItem(7, 1001, 152)
	if err != nil {
		t.Fatalf("wrong item: %v", err)
	}
	if applied || msg == "" {
		t.Fatalf("wrong item should be no-op with message, got applied=%v msg=%q", applied, msg)
	}
	var wrongQty int
	if err := conn.QueryRow(
		"SELECT quantity FROM user_items WHERE user_id = 7 AND item_id = 152",
	).Scan(&wrongQty); err != nil {
		t.Fatalf("read wrong item: %v", err)
	}
	if wrongQty != 1 {
		t.Fatalf("wrong item was consumed: qty=%d", wrongQty)
	}

	// Без предмета: у другого пользователя нет Up-Grade -> no-op.
	if _, err := conn.Exec(`
		INSERT INTO user_pokemons
			(id, user_id, pokemon_id, level, nature_id, current_hp, max_hp,
			 attack, defense, speed, special_attack, special_defense,
			 iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
			 ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense)
		VALUES (1002, 8, 137, 50, 0, 100, 100, 50, 50, 50, 50, 50,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	`); err != nil {
		t.Fatalf("insert no-item pokemon: %v", err)
	}
	applied, msg, err = EvolveWithItem(8, 1002, 155)
	if err != nil {
		t.Fatalf("no item: %v", err)
	}
	if applied || msg == "" {
		t.Fatalf("no item should be no-op with message, got applied=%v msg=%q", applied, msg)
	}
	var speciesAfter int
	if err := conn.QueryRow(
		"SELECT pokemon_id FROM user_pokemons WHERE id = 1002",
	).Scan(&speciesAfter); err != nil {
		t.Fatalf("read no-item species: %v", err)
	}
	if speciesAfter != 137 {
		t.Fatalf("no item evolved pokemon to %d", speciesAfter)
	}
}
