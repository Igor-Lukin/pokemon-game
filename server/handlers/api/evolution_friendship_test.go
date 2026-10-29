package api

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"pokemon-game/server/db"
)

// TestFriendshipEvolutionHappiness проверяет механику «дружба → счастье 255»:
//   - friendship-эволюция (method 4) без условия не срабатывает при счастье 254
//     и срабатывает ровно на 255;
//   - условные ветки (день/ночь) требуют И счастье 255, И своё условие;
//   - useHappiness: кламп на 255, корректный расход предметов, авто-эволюция в
//     момент достижения максимума, ошибки на max/чужого покемона.
func TestFriendshipEvolutionHappiness(t *testing.T) {
	oldDB := db.DB
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "evo_friendship.db"))
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
			evolution_level INTEGER, evolution_item_id INTEGER,
			evolution_condition TEXT, evolution_priority INTEGER DEFAULT 0)`,
		`CREATE TABLE user_pokemons (
			id INTEGER PRIMARY KEY, user_id INTEGER, pokemon_id INTEGER,
			level INTEGER, nature_id INTEGER, current_hp INTEGER, max_hp INTEGER,
			attack INTEGER, defense INTEGER, speed INTEGER,
			special_attack INTEGER, special_defense INTEGER,
			iv_hp INTEGER, iv_attack INTEGER, iv_defense INTEGER, iv_speed INTEGER,
			iv_special_attack INTEGER, iv_special_defense INTEGER,
			ev_hp INTEGER, ev_attack INTEGER, ev_defense INTEGER, ev_speed INTEGER,
			ev_special_attack INTEGER, ev_special_defense INTEGER,
			happiness INTEGER DEFAULT 0,
			ability_id INTEGER, ability_slot INTEGER)`,
		`CREATE TABLE natures (id INTEGER PRIMARY KEY, increased_stat TEXT, decreased_stat TEXT)`,
		`CREATE TABLE attacks (id INTEGER PRIMARY KEY, type_id INTEGER, pp INTEGER DEFAULT 10)`,
		`CREATE TABLE pokemon_attacks (
			pokemon_id INTEGER, attack_id INTEGER,
			learn_method_id INTEGER, level_learned_at INTEGER)`,
		`CREATE TABLE user_pokemon_learnable_attacks (
			user_pokemon_id INTEGER, attack_id INTEGER, level_available INTEGER,
			is_learned BOOLEAN DEFAULT 0, current_pp INTEGER DEFAULT 0,
			position INTEGER, source TEXT DEFAULT 'level', source_item_id INTEGER)`,
		`CREATE TABLE pokemon_abilities (
			pokemon_id INTEGER, ability_id INTEGER, is_hidden BOOLEAN, slot INTEGER)`,
	}
	for _, q := range schema {
		if _, err := conn.Exec(q); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	if _, err := conn.Exec("INSERT INTO natures (id, increased_stat, decreased_stat) VALUES (1, '', '')"); err != nil {
		t.Fatalf("nature: %v", err)
	}

	species := []struct {
		id                  int
		name                string
		from, method        int
		condition           string
	}{
		{133, "eevee", 0, 0, ""},
		{440, "happiny", 0, 0, ""},
		{113, "chansey", 440, 4, ""},
		{196, "espeon", 133, 4, evoConditionTimeDay},
	}
	for _, s := range species {
		var from, method, condition any
		if s.from > 0 {
			from = s.from
		}
		if s.method > 0 {
			method = s.method
		}
		if s.condition != "" {
			condition = s.condition
		}
		if _, err := conn.Exec(`
			INSERT INTO pokemons
				(id, name, image_url, base_hp, base_attack, base_defense,
				 base_speed, base_special_attack, base_special_defense,
				 evolves_from_id, evolution_method_id, evolution_condition,
				 evolution_priority)
			VALUES (?, ?, '', 50, 50, 50, 50, 50, 50, ?, ?, ?, 0)
		`, s.id, s.name, from, method, condition); err != nil {
			t.Fatalf("insert species %d: %v", s.id, err)
		}
	}

	insertUserPokemon := func(id, userID, pokemonID, happiness int) {
		if _, err := conn.Exec(`
			INSERT INTO user_pokemons
				(id, user_id, pokemon_id, level, nature_id, current_hp, max_hp,
				 attack, defense, speed, special_attack, special_defense,
				 iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
				 ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense,
				 happiness)
			VALUES (?, ?, ?, 50, 1, 100, 100, 50, 50, 50, 50, 50,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, ?)
		`, id, userID, pokemonID, happiness); err != nil {
			t.Fatalf("insert user pokemon %d: %v", id, err)
		}
	}

	insertUserPokemon(1, 7, 440, 254) // ниже порога
	insertUserPokemon(2, 7, 440, 255) // ровно порог
	insertUserPokemon(3, 7, 133, 255) // день -> Эспеон
	insertUserPokemon(4, 7, 440, 250) // ягода добивает до 255
	insertUserPokemon(5, 7, 440, 253) // избыток заявки -> расход 1
	insertUserPokemon(6, 7, 440, 255) // уже максимум
	insertUserPokemon(7, 8, 440, 100) // чужой покемон

	oldNow := evolutionNow
	defer func() { evolutionNow = oldNow }()
	evolutionNow = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }

	// Порог: 254 — нет, 255 — да.
	info, err := checkEvolutionFor(1, 440, 50)
	if err != nil {
		t.Fatalf("happiness 254: %v", err)
	}
	if info.CanEvolve {
		t.Fatalf("happiness 254 must not evolve, got %d", info.EvolvesToID)
	}
	info, err = checkEvolutionFor(2, 440, 50)
	if err != nil {
		t.Fatalf("happiness 255: %v", err)
	}
	if !info.CanEvolve || info.EvolvesToID != 113 {
		t.Fatalf("happiness 255 must evolve to 113, got can=%v id=%d", info.CanEvolve, info.EvolvesToID)
	}

	// Условная ветка: счастье 255 + день -> Эспеон.
	info, err = checkEvolutionFor(3, 133, 50)
	if err != nil {
		t.Fatalf("espeon day: %v", err)
	}
	if !info.CanEvolve || info.EvolvesToID != 196 {
		t.Fatalf("day + happiness 255 must evolve Eevee to 196, got can=%v id=%d", info.CanEvolve, info.EvolvesToID)
	}
	evolutionNow = func() time.Time { return time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC) }
	info, err = checkEvolutionFor(3, 133, 50)
	if err != nil {
		t.Fatalf("espeon night: %v", err)
	}
	if info.CanEvolve {
		t.Fatalf("night must not evolve Eevee to Espeon, got %d", info.EvolvesToID)
	}

	// useHappiness: 250 + 5 -> 255, авто-эволюция Хаппини в Ченси.
	msg, applied, err := useHappiness(7, 4, 5, 1)
	if err != nil {
		t.Fatalf("useHappiness 250: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied=%d, want 1", applied)
	}
	var speciesID, happiness int
	if err := conn.QueryRow("SELECT pokemon_id, happiness FROM user_pokemons WHERE id = 4").
		Scan(&speciesID, &happiness); err != nil {
		t.Fatalf("read pokemon 4: %v", err)
	}
	if happiness != 255 || speciesID != 113 {
		t.Fatalf("got species=%d happiness=%d, want 113/255 (msg=%q)", speciesID, happiness, msg)
	}

	// Избыточная заявка: 253 + 5 -> 255, потрачена 1 ягода, не 10.
	_, applied, err = useHappiness(7, 5, 5, 10)
	if err != nil {
		t.Fatalf("useHappiness overflow: %v", err)
	}
	if applied != 1 {
		t.Fatalf("overflow applied=%d, want 1", applied)
	}
	if err := conn.QueryRow("SELECT happiness FROM user_pokemons WHERE id = 5").Scan(&happiness); err != nil {
		t.Fatalf("read pokemon 5: %v", err)
	}
	if happiness != 255 {
		t.Fatalf("overflow happiness=%d, want 255", happiness)
	}

	// Уже максимум — ошибка, ничего не тратится.
	if _, applied, err = useHappiness(7, 6, 5, 1); err == nil {
		t.Fatalf("useHappiness at max must error, applied=%d", applied)
	}
	// Чужой покемон — ошибка доступа.
	if _, _, err = useHappiness(7, 7, 5, 1); err != errPermission {
		t.Fatalf("useHappiness foreign owner err=%v, want errPermission", err)
	}
}
