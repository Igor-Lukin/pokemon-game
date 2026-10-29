package api

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"pokemon-game/server/db"
)

func TestEvolutionConditionMet(t *testing.T) {
	oldNow := evolutionNow
	defer func() { evolutionNow = oldNow }()

	ctx := &evolutionContext{attack: 40, defense: 30, knowsFairyMove: false}

	evolutionNow = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	if !evolutionConditionMet(evoConditionTimeDay, ctx) {
		t.Fatal("time_day must be true at 12:00")
	}
	if evolutionConditionMet(evoConditionTimeNight, ctx) {
		t.Fatal("time_night must be false at 12:00")
	}

	evolutionNow = func() time.Time { return time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC) }
	if !evolutionConditionMet(evoConditionTimeNight, ctx) {
		t.Fatal("time_night must be true at 23:00")
	}
	if evolutionConditionMet(evoConditionTimeDay, ctx) {
		t.Fatal("time_day must be false at 23:00")
	}

	if !evolutionConditionMet(evoConditionAtkGtDef, ctx) {
		t.Fatal("atk_gt_def must match 40/30")
	}
	if evolutionConditionMet(evoConditionAtkLtDef, ctx) || evolutionConditionMet(evoConditionAtkEqDef, ctx) {
		t.Fatal("wrong stat condition matched 40/30")
	}
	ctx.attack, ctx.defense = 30, 40
	if !evolutionConditionMet(evoConditionAtkLtDef, ctx) {
		t.Fatal("atk_lt_def must match 30/40")
	}
	ctx.attack, ctx.defense = 35, 35
	if !evolutionConditionMet(evoConditionAtkEqDef, ctx) {
		t.Fatal("atk_eq_def must match 35/35")
	}

	ctx.knowsFairyMove = true
	if !evolutionConditionMet(evoConditionFairyMove, ctx) {
		t.Fatal("fairy_move must match when the move is known")
	}
	if evolutionConditionMet("typo_condition", ctx) {
		t.Fatal("unknown condition must be false")
	}
}

// TestCheckEvolutionBranches проверяет детерминированный выбор веток Иви и
// Тайрога на изолированной БД.
func TestCheckEvolutionBranches(t *testing.T) {
	oldDB := db.DB
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "evo_branches.db"))
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
			happiness INTEGER DEFAULT 0)`,
		`CREATE TABLE natures (id INTEGER PRIMARY KEY, increased_stat TEXT, decreased_stat TEXT)`,
		`CREATE TABLE attacks (id INTEGER PRIMARY KEY, type_id INTEGER)`,
		`CREATE TABLE user_pokemon_learnable_attacks (
			user_pokemon_id INTEGER, attack_id INTEGER, is_learned BOOLEAN DEFAULT 0)`,
	}
	for _, q := range schema {
		if _, err := conn.Exec(q); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	if _, err := conn.Exec("INSERT INTO natures (id, increased_stat, decreased_stat) VALUES (1, '', '')"); err != nil {
		t.Fatalf("nature: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO attacks (id, type_id) VALUES (405, 18)"); err != nil {
		t.Fatalf("attack: %v", err)
	}

	species := []struct {
		id                        int
		name                      string
		from, method, level, prio int
		condition                 string
	}{
		{133, "eevee", 0, 0, 0, 0, ""},
		{196, "espeon", 133, 4, 0, 0, evoConditionTimeDay},
		{197, "umbreon", 133, 4, 0, 0, evoConditionTimeNight},
		{700, "sylveon", 133, 4, 0, 10, evoConditionFairyMove},
		{236, "tyrogue", 0, 0, 0, 0, ""},
		{106, "hitmonlee", 236, 1, 20, 0, evoConditionAtkGtDef},
		{107, "hitmonchan", 236, 1, 20, 0, evoConditionAtkLtDef},
		{237, "hitmontop", 236, 1, 20, 0, evoConditionAtkEqDef},
	}
	for _, s := range species {
		var from, method, level, condition any
		if s.from > 0 {
			from = s.from
		}
		if s.method > 0 {
			method = s.method
		}
		if s.level > 0 {
			level = s.level
		}
		if s.condition != "" {
			condition = s.condition
		}
		if _, err := conn.Exec(`
			INSERT INTO pokemons
				(id, name, image_url, base_hp, base_attack, base_defense,
				 base_speed, base_special_attack, base_special_defense,
				 evolves_from_id, evolution_method_id, evolution_level,
				 evolution_condition, evolution_priority)
			VALUES (?, ?, '', 50, 50, 50, 50, 50, 50, ?, ?, ?, ?, ?)
		`, s.id, s.name, from, method, level, condition, s.prio); err != nil {
			t.Fatalf("insert species %d: %v", s.id, err)
		}
	}

	insertUserPokemon := func(id, pokemonID, level, ivAtk, ivDef int) {
		if _, err := conn.Exec(`
			INSERT INTO user_pokemons
				(id, user_id, pokemon_id, level, nature_id, current_hp, max_hp,
				 attack, defense, speed, special_attack, special_defense,
				 iv_hp, iv_attack, iv_defense, iv_speed, iv_special_attack, iv_special_defense,
				 ev_hp, ev_attack, ev_defense, ev_speed, ev_special_attack, ev_special_defense,
				 happiness)
			VALUES (?, 7, ?, ?, 1, 100, 100, 50, 50, 50, 50, 50,
				0, ?, ?, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255)
		`, id, pokemonID, level, ivAtk, ivDef); err != nil {
			t.Fatalf("insert user pokemon %d: %v", id, err)
		}
	}

	insertUserPokemon(1, 133, 10, 0, 0) // Иви без Fairy-атаки
	insertUserPokemon(2, 236, 20, 31, 0)
	insertUserPokemon(3, 236, 20, 0, 31)
	insertUserPokemon(4, 236, 20, 0, 0)
	insertUserPokemon(5, 133, 10, 0, 0) // Иви с Fairy-атакой
	if _, err := conn.Exec(`
		INSERT INTO user_pokemon_learnable_attacks (user_pokemon_id, attack_id, is_learned)
		VALUES (5, 405, 1)
	`); err != nil {
		t.Fatalf("learn fairy move: %v", err)
	}

	oldNow := evolutionNow
	defer func() { evolutionNow = oldNow }()

	expect := func(userPokemonID, pokemonID, level, want int, label string) {
		t.Helper()
		for i := 0; i < 20; i++ {
			info, err := checkEvolutionFor(userPokemonID, pokemonID, level)
			if err != nil {
				t.Fatalf("%s: %v", label, err)
			}
			if !info.CanEvolve || info.EvolvesToID != want {
				t.Fatalf("%s: got can=%v id=%d, want %d", label, info.CanEvolve, info.EvolvesToID, want)
			}
		}
	}

	evolutionNow = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	expect(1, 133, 10, 196, "Eevee day -> Espeon")
	expect(5, 133, 10, 700, "Eevee with Fairy move -> Sylveon")
	expect(2, 236, 20, 106, "Tyrogue Atk>Def -> Hitmonlee")
	expect(3, 236, 20, 107, "Tyrogue Atk<Def -> Hitmonchan")
	expect(4, 236, 20, 237, "Tyrogue Atk=Def -> Hitmontop")

	evolutionNow = func() time.Time { return time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC) }
	expect(1, 133, 10, 197, "Eevee night -> Umbreon")
}
