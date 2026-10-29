package db

import "log"

// pokemon_forms.go — данные боевых/альтернативных форм покемонов.
//
// Строки таблицы описывают НЕ базовую форму вида (она лежит в pokemons), а
// переключаемые формы. Движок сам не знает конкретных видов: он читает правила
// отсюда и переключает имя/спрайт/типы/статы.
//
// trigger_key        — effect_key способности, управляющей формой;
// trigger_mode       — 'hp' (порог HP) или 'move' (по использованному ходу);
// trigger_move_id    — для 'move': конкретный id атаки (0 = любая по kind);
// trigger_move_kind  — для 'move': 'damaging' / 'status' / '' (любая);
// is_base_form       — 1: форма совпадает с базовой (используется для возврата);
// form_label         — короткая подпись для покедекса;
// *_mult             — множители боевых статов формы (1.0 = без изменений).

// BattleForm — правило одной формы (строка pokemon_forms).
type BattleForm struct {
	PokemonID          int
	FormKey            string
	TriggerKey         string
	TriggerMode        string
	TriggerMoveID      int
	TriggerMoveKind    string
	FormName           string
	FormLabel          string
	ImageURL           string
	PrimaryTypeID      int
	SecondaryTypeID    int
	HPThresholdPct     int
	HPAbove            bool
	MinLevel           int
	IsBaseForm         bool
	FormOrder          int
	FormDescription    string
	AttackMult         float64
	DefenseMult        float64
	SpeedMult          float64
	SpecialAttackMult  float64
	SpecialDefenseMult float64
}

const formSelectColumns = `
	form_key, form_name, COALESCE(form_label, ''), COALESCE(image_url, ''),
	COALESCE(primary_type_id, 0), COALESCE(secondary_type_id, 0),
	hp_threshold_percent, hp_mode, COALESCE(min_level, 0),
	COALESCE(attack_mult, 1), COALESCE(defense_mult, 1),
	COALESCE(speed_mult, 1), COALESCE(special_attack_mult, 1),
	COALESCE(special_defense_mult, 1),
	COALESCE(trigger_mode, 'hp'), COALESCE(trigger_move_id, 0),
	COALESCE(trigger_move_kind, ''), COALESCE(is_base_form, 0),
	COALESCE(form_order, 0), COALESCE(form_description, '')`

func scanForm(row interface{ Scan(...interface{}) error }, pokemonID int, triggerKey string) (*BattleForm, bool) {
	var f BattleForm
	var hpMode, isBase int
	if row.Scan(&f.FormKey, &f.FormName, &f.FormLabel, &f.ImageURL,
		&f.PrimaryTypeID, &f.SecondaryTypeID,
		&f.HPThresholdPct, &hpMode, &f.MinLevel,
		&f.AttackMult, &f.DefenseMult, &f.SpeedMult,
		&f.SpecialAttackMult, &f.SpecialDefenseMult,
		&f.TriggerMode, &f.TriggerMoveID, &f.TriggerMoveKind,
		&isBase, &f.FormOrder, &f.FormDescription) != nil {
		return nil, false
	}
	f.PokemonID = pokemonID
	f.TriggerKey = triggerKey
	f.HPAbove = hpMode != 0
	f.IsBaseForm = isBase != 0
	return &f, true
}

// FormFor — подобрать активную HP-форму вида по способности, уровню и HP%.
// Возвращает ok=false, если ни одно правило не подходит (значит, базовая форма).
func FormFor(pokemonID int, triggerKey string, level, hpPercent int) (*BattleForm, bool) {
	rows, err := DB.Query(`
		SELECT `+formSelectColumns+`
		FROM pokemon_forms
		WHERE pokemon_id = ? AND trigger_key = ? AND COALESCE(trigger_mode, 'hp') = 'hp'
		ORDER BY form_key`, pokemonID, triggerKey)
	if err != nil {
		log.Printf("FormFor: query error pokemon=%d key=%s: %v", pokemonID, triggerKey, err)
		return nil, false
	}
	defer rows.Close()
	for rows.Next() {
		f, ok := scanForm(rows, pokemonID, triggerKey)
		if !ok {
			continue
		}
		if level < f.MinLevel {
			continue
		}
		if f.HPAbove {
			if hpPercent <= f.HPThresholdPct {
				continue
			}
		} else if hpPercent > f.HPThresholdPct {
			continue
		}
		return f, true
	}
	return nil, false
}

// FormForMove — подобрать форму, переключаемую использованным ходом.
// moveID/categoryID/power берутся у применённой атаки.
func FormForMove(pokemonID int, triggerKey string, moveID, categoryID, power int) (*BattleForm, bool) {
	rows, err := DB.Query(`
		SELECT `+formSelectColumns+`
		FROM pokemon_forms
		WHERE pokemon_id = ? AND trigger_key = ? AND COALESCE(trigger_mode, 'hp') = 'move'
		ORDER BY is_base_form, form_order, form_key`, pokemonID, triggerKey)
	if err != nil {
		log.Printf("FormForMove: query error pokemon=%d key=%s: %v", pokemonID, triggerKey, err)
		return nil, false
	}
	defer rows.Close()
	for rows.Next() {
		f, ok := scanForm(rows, pokemonID, triggerKey)
		if !ok {
			continue
		}
		if f.TriggerMoveID > 0 && f.TriggerMoveID != moveID {
			continue
		}
		if f.TriggerMoveID == 0 {
			switch f.TriggerMoveKind {
			case "damaging":
				if categoryID == 3 || power <= 0 {
					continue
				}
			case "status":
				if categoryID != 3 {
					continue
				}
			}
		}
		return f, true
	}
	return nil, false
}

// FormsForPokemon — все формы вида (базовая + альтернативные) для покедекса.
func FormsForPokemon(pokemonID int) []BattleForm {
	rows, err := DB.Query(`
		SELECT `+formSelectColumns+`
		FROM pokemon_forms
		WHERE pokemon_id = ?
		ORDER BY is_base_form DESC, form_order, form_key`, pokemonID)
	if err != nil {
		log.Printf("FormsForPokemon: query error pokemon=%d: %v", pokemonID, err)
		return nil
	}
	defer rows.Close()
	forms := make([]BattleForm, 0)
	for rows.Next() {
		f, ok := scanForm(rows, pokemonID, "")
		if !ok {
			continue
		}
		forms = append(forms, *f)
	}
	return forms
}

// SeedPokemonForms — данные боевых форм (идемпотентно). Для текущей БД
// значения уже могут быть поправлены вручную; INSERT OR IGNORE не перезаписывает.
func SeedPokemonForms() {
	DB.Exec(`UPDATE abilities SET effect_key = 'stance_change'
		WHERE id = 86 AND (effect_key IS NULL OR effect_key = '')`)

	DB.Exec(`INSERT OR IGNORE INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, image_url,
		 primary_type_id, secondary_type_id,
		 hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order, form_description)
		VALUES (681, 'shield', 'stance_change', 'Иджислаш', 'Защитная форма',
			'/static/images/pokemons/normal/681.gif', 17, 14,
			50, 1, 0, 1, 1, 1, 1, 1,
			'move', 404, '', 1, 0,
			'Защитная форма: Иджислаш прикрывается щитом и лучше держит удары.')`)

	DB.Exec(`INSERT OR IGNORE INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, image_url,
		 primary_type_id, secondary_type_id,
		 hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order, form_description)
		VALUES (681, 'blade', 'stance_change', 'Иджислаш (Атакующая форма)', 'Атакующая форма',
			'/static/images/pokemons/normal/681-blade.gif', 17, 14,
			50, 1, 0, 3, 0.357142857, 1, 3, 0.357142857,
			'move', 0, 'damaging', 0, 1,
			'Атакующая форма: Иджислаш раскрывает клинок и меняет защиту на мощную атаку.')`)
}
