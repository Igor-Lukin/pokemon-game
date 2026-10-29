package api

import (
	"testing"

	"pokemon-game/server/db"
)

func TestPokedexFormsBaseAndBlade(t *testing.T) {
	if _, err := db.DB.Exec(`INSERT INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, image_url,
		 primary_type_id, secondary_type_id, hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order)
		VALUES (6820, 'shield', 'stance_change', 'Иджислаш', 'Защитная форма',
		 '/static/images/pokemons/normal/6820.gif', 17, 14, 50, 1, 0,
		 1, 1, 1, 1, 1, 'move', 404, '', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, image_url,
		 primary_type_id, secondary_type_id, hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order)
		VALUES (6820, 'blade', 'stance_change', 'Иджислаш (Атакующая форма)', 'Атакующая форма',
		 '/static/images/pokemons/normal/6820-blade.gif', 17, 14, 50, 1, 0,
		 3, 0.357142857, 1, 3, 0.357142857, 'move', 0, 'damaging', 0, 1)`); err != nil {
		t.Fatal(err)
	}

	info := &PokemonFullInfo{
		ID: 6820, Name: "Иджислаш", Description: "base", ImageURL: "/base.gif",
		PrimaryType: "Steel", SecondaryType: "Ghost", PrimaryTypeID: 17, SecondaryTypeID: intPtr(14),
		BaseStats: map[string]int{"hp": 60, "attack": 50, "defense": 140, "speed": 60, "special_attack": 50, "special_defense": 140},
	}
	applyPokemonForms(info, "")
	if len(info.Forms) != 2 || !info.Forms[0].IsBase || info.Forms[1].Key != "blade" {
		t.Fatalf("forms list: %+v", info.Forms)
	}
	if info.FormName != "Защитная форма" || info.FormKey != "" {
		t.Fatalf("base form label: key=%q name=%q", info.FormKey, info.FormName)
	}

	info = &PokemonFullInfo{
		ID: 6820, Name: "Иджислаш", Description: "base", ImageURL: "/base.gif",
		PrimaryType: "Steel", SecondaryType: "Ghost", PrimaryTypeID: 17, SecondaryTypeID: intPtr(14),
		BaseStats: map[string]int{"hp": 60, "attack": 50, "defense": 140, "speed": 60, "special_attack": 50, "special_defense": 140},
	}
	applyPokemonForms(info, "blade")
	if info.FormKey != "blade" || info.FormName != "Атакующая форма" {
		t.Fatalf("selected form: key=%q name=%q", info.FormKey, info.FormName)
	}
	if info.ImageURL != "/static/images/pokemons/normal/6820-blade.gif" {
		t.Fatalf("image: %q", info.ImageURL)
	}
	if info.BaseStats["attack"] != 150 || info.BaseStats["defense"] != 50 ||
		info.BaseStats["special_attack"] != 150 || info.BaseStats["special_defense"] != 50 {
		t.Fatalf("stats: %+v", info.BaseStats)
	}
}

func intPtr(v int) *int { return &v }
