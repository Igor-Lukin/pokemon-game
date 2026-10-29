package api

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"pokemon-game/server/db"
)

// TestMain — минимальная in-memory БД: нужна только таблица type_effectiveness
// (для calculateDamage) и pokemon_forms (для форм по HP). Реальные данные не
// читаются и не пишутся.
func TestMain(m *testing.M) {
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	db.DB = conn
	conn.Exec(`CREATE TABLE type_effectiveness (
		attacking_type_id INTEGER, defending_type_id INTEGER, multiplier REAL)`)
	conn.Exec(`CREATE TABLE pokemon_forms (
		pokemon_id INTEGER, form_key TEXT, trigger_key TEXT, form_name TEXT,
		image_url TEXT, primary_type_id INTEGER, secondary_type_id INTEGER,
		hp_threshold_percent INTEGER, hp_mode INTEGER, min_level INTEGER,
		attack_mult REAL, defense_mult REAL, speed_mult REAL,
		special_attack_mult REAL, special_defense_mult REAL,
		form_label TEXT, trigger_mode TEXT, trigger_move_id INTEGER,
		trigger_move_kind TEXT, is_base_form INTEGER, form_order INTEGER,
		form_description TEXT)`)
	code := m.Run()
	conn.Close()
	os.Exit(code)
}

// Точечные проверки механик батча 5 без БД и без ГСЧ: все ветки детерминированы.

func newTestPokemon(name string, hp int) *BattlePokemon {
	return &BattlePokemon{
		Name: name, MaxHP: hp, CurrentHP: hp,
		PrimaryTypeID: 1, Attack: 100, Defense: 100, Speed: 100,
		SpecialAttack: 100, SpecialDefense: 100, Level: 50,
	}
}

func hasMsg(log []BattleLogEntry, want string) bool {
	for _, e := range log {
		if e.Message == want {
			return true
		}
	}
	return false
}

func TestAbilityAbsorbsStatusMove(t *testing.T) {
	cases := []struct {
		name      string
		ability   string
		move      BattleMove
		want      bool
		wantHP    int
		wantBoost bool
	}{
		{
			name: "flash_fire fire status", ability: "flash_fire",
			move: BattleMove{Name: "Испепеление", TypeID: 2, CategoryID: 3, TargetID: 1, EffectID: 3},
			want: true, wantBoost: true,
		},
		{
			name: "flash_fire weather move excluded", ability: "flash_fire",
			move: BattleMove{Name: "Ясный день", TypeID: 2, CategoryID: 3, TargetID: 1, EffectID: 14},
			want: false,
		},
		{
			name: "self target excluded", ability: "volt_absorb",
			move: BattleMove{Name: "Заряд", TypeID: 4, CategoryID: 3, TargetID: 2},
			want: false,
		},
		{
			name: "water_absorb water status heals", ability: "water_absorb",
			move: BattleMove{Name: "Увлажнение", TypeID: 3, CategoryID: 3, TargetID: 1},
			want: true, wantHP: 65,
		},
		{
			name: "dry_skin water status heals", ability: "dry_skin",
			move: BattleMove{Name: "Водный покров", TypeID: 3, CategoryID: 3, TargetID: 1},
			want: true, wantHP: 65,
		},
		{
			name: "sap_sipper grass status", ability: "sap_sipper",
			move: BattleMove{Name: "Семена-пиявки", TypeID: 5, CategoryID: 3, TargetID: 1},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bs := &BattleState{}
			atk := newTestPokemon("Атакующий", 100)
			def := newTestPokemon("Носитель", 100)
			def.CurrentHP = 40
			def.AbilityKey = tc.ability
			var log []BattleLogEntry
			got := abilityAbsorbsStatusMove(bs, atk, def, &tc.move, 1, &log)
			if got != tc.want {
				t.Fatalf("absorbed=%v, want %v", got, tc.want)
			}
			if tc.wantHP != 0 && def.CurrentHP != tc.wantHP {
				t.Fatalf("HP=%d, want %d", def.CurrentHP, tc.wantHP)
			}
			if tc.wantBoost && !def.FlashFireBoosted {
				t.Fatal("FlashFireBoosted не выставлен")
			}
			if got && len(log) == 0 {
				t.Fatal("поглощение без записи в лог")
			}
		})
	}
}

func TestPoisonHealTick(t *testing.T) {
	for _, status := range []int{6, 7} {
		p := newTestPokemon("Токсифаг", 160)
		p.CurrentHP = 100
		p.StatusID = status
		p.AbilityKey = "poison_heal"
		if status == 7 {
			p.PoisonTurns = 1
		}
		var log []BattleLogEntry
		if !processPreTurnStatus(p, &log) {
			t.Fatalf("status %d: покемон не должен терять ход", status)
		}
		if p.CurrentHP != 120 {
			t.Fatalf("status %d: HP=%d, want 120 (лечение 1/8)", status, p.CurrentHP)
		}
		if p.IsFainted {
			t.Fatalf("status %d: покемон не должен падать", status)
		}
		found := false
		for _, e := range log {
			if e.Message != "" {
				found = true
			}
		}
		if !found {
			t.Fatalf("status %d: нет лога лечения", status)
		}
		if status == 7 && p.PoisonTurns != 2 {
			t.Fatalf("сильный яд: PoisonTurns=%d, want 2", p.PoisonTurns)
		}
	}
}

func TestWeatherMultiHooks(t *testing.T) {
	sun := &BattleState{BattleWeatherID: 2}
	rain := &BattleState{BattleWeatherID: 3}
	solar := newTestPokemon("Солнечная", 160)
	solar.AbilityKey = "solar_power"
	dry := newTestPokemon("Сухая", 160)
	dry.AbilityKey = "dry_skin"

	special := &BattleMove{TypeID: 2, CategoryID: 2}
	if m := abilityModifyOutgoingDamage(sun, solar, nil, special, 1, nil); m != 1.5 {
		t.Fatalf("Solar Power спец. в солнце = %v, want 1.5", m)
	}
	if m := abilityModifyOutgoingDamage(rain, solar, nil, special, 1, nil); m != 1.0 {
		t.Fatalf("Solar Power без солнца = %v, want 1.0", m)
	}
	if m := abilityModifyIncomingDamage(sun, nil, dry, &BattleMove{TypeID: 2, CategoryID: 2}, 1, nil); m != 1.25 {
		t.Fatalf("Dry Skin от огня = %v, want 1.25", m)
	}

	var log []BattleLogEntry
	abilityOnEndTurn(sun, solar, 1, &log)
	if solar.CurrentHP != 140 {
		t.Fatalf("Solar Power в солнце HP=%d, want 140", solar.CurrentHP)
	}
	abilityOnEndTurn(rain, dry, 2, &log)
	if dry.CurrentHP != 160 {
		t.Fatalf("Dry Skin в дождь HP=%d, want 160 (лечение с клампом)", dry.CurrentHP)
	}
	dry.CurrentHP = 100
	abilityOnEndTurn(sun, dry, 3, &log)
	if dry.CurrentHP != 80 {
		t.Fatalf("Dry Skin в солнце HP=%d, want 80", dry.CurrentHP)
	}
	if len(log) == 0 {
		t.Fatal("погодные способности не пишут в лог")
	}
}

func TestWeakArmorPhysicalOnly(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Броня", 100)
	def.AbilityKey = "weak_armor"
	var log []BattleLogEntry

	abilityAfterBeingHit(bs, atk, def, &BattleMove{CategoryID: 1, Contact: false}, 1, &log)
	if def.StatStages[2] != -1 || def.StatStages[3] != 2 {
		t.Fatalf("Weak Armor: Def=%d Speed=%d, want -1/+2", def.StatStages[2], def.StatStages[3])
	}
	def.StatStages[2], def.StatStages[3] = 0, 0
	abilityAfterBeingHit(bs, atk, def, &BattleMove{CategoryID: 2}, 2, &log)
	if def.StatStages[2] != 0 || def.StatStages[3] != 0 {
		t.Fatal("Weak Armor сработала на спец. атаку")
	}
}

func TestWeatherEvasionAndLevitate(t *testing.T) {
	sand := newTestPokemon("Пескоструй", 100)
	sand.AbilityKey = "sand_veil"
	if b := abilityEvasionBonus(sand, 5); b != 1 {
		t.Fatalf("sand_veil в бурю = %d, want 1", b)
	}
	if b := abilityEvasionBonus(sand, 4); b != 0 {
		t.Fatalf("sand_veil в град = %d, want 0", b)
	}
	snow := newTestPokemon("Белотелый", 100)
	snow.AbilityKey = "snow_cloak"
	if b := abilityEvasionBonus(snow, 4); b != 1 {
		t.Fatalf("snow_cloak в град = %d, want 1", b)
	}
	lev := newTestPokemon("Левитация", 100)
	lev.AbilityKey = "levitate"
	if !abilityHovering(lev, &BattleState{}) {
		t.Fatal("Левитация не парит")
	}
	if abilityHovering(lev, &BattleState{GravityTurns: 5}) {
		t.Fatal("Левитация должна теряться под Гравитацией")
	}
	atk := newTestPokemon("Атакующий", 100)
	move := &BattleMove{ID: 74, TypeID: 9, CategoryID: 1, Power: 100, Accuracy: 100}
	dmg, _, eff := calculateDamage(&BattleState{}, atk, lev, move, nil, 1, nil)
	if eff != 0 || dmg != 0 {
		t.Fatalf("Левитация: dmg=%d eff=%v, want 0/0", dmg, eff)
	}
}

func TestAbilityFormByHP(t *testing.T) {
	p := newTestPokemon("Базовая", 100)
	p.PrimaryTypeID = 1
	p.AbilityKey = "schooling"
	p.Attack = 100
	p.SpecialAttack = 50
	form := &db.BattleForm{
		FormKey: "school", FormName: "Стайная", PrimaryTypeID: 3,
		AttackMult: 2, SpecialAttackMult: 1.5,
		DefenseMult: 1, SpeedMult: 1, SpecialDefenseMult: 1,
	}
	var log []BattleLogEntry
	applyAbilityForm(p, form, 1, &log)
	if p.FormKey != "school" || p.Name != "Стайная" || p.PrimaryTypeID != 3 {
		t.Fatalf("форма не применилась: %+v", p)
	}
	if p.Attack != 200 || p.SpecialAttack != 75 {
		t.Fatalf("статы формы: Atk=%d SpA=%d, want 200/75", p.Attack, p.SpecialAttack)
	}
	resetAbilityForm(p, 2, &log)
	if p.FormKey != "" || p.Name != "Базовая" || p.PrimaryTypeID != 1 {
		t.Fatalf("форма не откатилась: %+v", p)
	}
	if p.Attack != 100 || p.SpecialAttack != 50 {
		t.Fatalf("статы после отката: Atk=%d SpA=%d", p.Attack, p.SpecialAttack)
	}
	if !abilityUsesHPForm("shields_down") || !abilityUsesHPForm("totem") || abilityUsesHPForm("weak_armor") {
		t.Fatal("неверный гейт форм по HP")
	}
}

// TestBatch1to4Regression — выборочная регрессия уже сданных батчей 1–4:
// ключевые хуки должны остаться нетронутыми правками батча 5.
func TestBatch1to4Regression(t *testing.T) {
	bs := &BattleState{BattleWeatherID: 3}
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Защитник", 100)

	atk.AbilityKey, atk.CurrentHP = "blaze", 10
	if m := abilityModifyOutgoingDamage(bs, atk, def, &BattleMove{TypeID: 2}, 1, nil); m != 1.5 {
		t.Fatalf("blaze pinch = %v, want 1.5", m)
	}
	def.AbilityKey = "thick_fat"
	if m := abilityModifyIncomingDamage(bs, atk, def, &BattleMove{TypeID: 6}, 1, nil); m != 0.5 {
		t.Fatalf("thick_fat = %v, want 0.5", m)
	}
	def.AbilityKey = "immunity"
	if !abilityBlocksStatus(bs, def, 6) || abilityBlocksStatus(bs, def, 2) {
		t.Fatal("immunity сломан")
	}
	def.AbilityKey = "rain_dish"
	def.CurrentHP = 80
	var log []BattleLogEntry
	abilityOnEndTurn(bs, def, 1, &log)
	if def.CurrentHP <= 80 {
		t.Fatal("rain_dish не лечит в дождь")
	}
	def.AbilityKey = "speed_boost"
	abilityOnEndTurn(bs, def, 2, &log)
	if def.StatStages[3] != 1 {
		t.Fatal("speed_boost не повышает Скорость")
	}
	def.AbilityKey = "clear_body"
	if !abilityBlocksStatDrop(def, 1) {
		t.Fatal("clear_body сломан")
	}
	atk.AbilityKey = "compound_eyes"
	if m := abilityAccuracyMult(atk, def, &BattleMove{CategoryID: 1}); m != 1.3 {
		t.Fatalf("compound_eyes = %v, want 1.3", m)
	}
	def.AbilityKey = "battle_armor"
	if !abilityBlocksCrit(def) {
		t.Fatal("battle_armor сломан")
	}
	def.AbilityKey = "flash_fire"
	def.CurrentHP = 50
	if !abilityAbsorbsMove(bs, atk, def, &BattleMove{TypeID: 2, CategoryID: 2}, 1, &log) {
		t.Fatal("flash_fire больше не поглощает огонь")
	}
	def.AbilityKey = "rough_skin"
	atk.CurrentHP = 100
	abilityOnHitByMove(bs, atk, def, &BattleMove{Contact: true}, 1, &log)
	if atk.CurrentHP != 88 {
		t.Fatalf("rough_skin: HP атакующего=%d, want 88", atk.CurrentHP)
	}
}

// --- Точечные "бои" через реальный оркестратор processTurn (без HTTP) ---

func battleStateFor(player, wild *BattlePokemon) *BattleState {
	player.Speed, wild.Speed = 200, 1
	bs := &BattleState{
		PlayerTeam: []BattlePokemon{*player}, ActiveIdx: 0,
		WildPokemon: wild, BattleType: "wild", BattleWeatherID: 1,
	}
	return bs
}

func TestBattleStatusAbsorbTurn(t *testing.T) {
	player := newTestPokemon("Игрок", 100)
	player.Moves = []BattleMove{{
		ID: 999, Name: "Электрошок", TypeID: 4, CategoryID: 3, TargetID: 1,
		EffectID: 2, EffectChance: 100, Accuracy: 100, PP: 10, MaxPP: 10,
	}}
	wild := newTestPokemon("Дикий", 100)
	wild.CurrentHP = 40
	wild.AbilityKey = "volt_absorb"
	wild.Moves = []BattleMove{{ID: 1, Name: "Толчок", TypeID: 1, CategoryID: 1, Power: 20, Accuracy: 100, PP: -1, MaxPP: -1}}
	bs := battleStateFor(player, wild)
	processTurn(bs, &bs.PlayerTeam[0].Moves[0], 0)
	if wild.StatusID == 2 {
		t.Fatal("Электрошок наложил паралич на Аккумулятор")
	}
	if wild.CurrentHP != 65 {
		t.Fatalf("Аккумулятор HP=%d, want 65 (поглощение + лечение ¼)", wild.CurrentHP)
	}
}

func TestBattleWeakArmorTurn(t *testing.T) {
	player := newTestPokemon("Игрок", 100)
	player.Attack = 200
	player.Moves = []BattleMove{{
		ID: 2, Name: "Камнепад", TypeID: 13, CategoryID: 1, Power: 30,
		Accuracy: 100, Contact: false, PP: 10, MaxPP: 10,
	}}
	wild := newTestPokemon("Дикий", 100)
	wild.Defense = 200
	wild.AbilityKey = "weak_armor"
	wild.Moves = []BattleMove{{ID: 3, Name: "Ожидание", TypeID: 1, CategoryID: 3, TargetID: 2, PP: -1, MaxPP: -1}}
	bs := battleStateFor(player, wild)
	processTurn(bs, &bs.PlayerTeam[0].Moves[0], 0)
	if wild.StatStages[2] != -1 || wild.StatStages[3] != 2 {
		t.Fatalf("Weak Armor после физ. удара: Def=%d Speed=%d, want -1/+2", wild.StatStages[2], wild.StatStages[3])
	}
}

func TestBattlePoisonHealTurn(t *testing.T) {
	player := newTestPokemon("Игрок", 160)
	player.CurrentHP = 80
	player.StatusID = 6
	player.AbilityKey = "poison_heal"
	player.Moves = []BattleMove{{ID: 4, Name: "Ожидание", TypeID: 1, CategoryID: 3, TargetID: 2, PP: 10, MaxPP: 10}}
	wild := newTestPokemon("Дикий", 100)
	wild.Moves = []BattleMove{{ID: 5, Name: "Ожидание", TypeID: 1, CategoryID: 3, TargetID: 2, PP: -1, MaxPP: -1}}
	bs := battleStateFor(player, wild)
	processTurn(bs, &bs.PlayerTeam[0].Moves[0], 0)
	if got := bs.PlayerTeam[0].CurrentHP; got != 100 {
		t.Fatalf("Токсифаг HP=%d, want 100 (+1/8 за тик яда)", got)
	}
}

func TestAbilityLogFormatAndThrottle(t *testing.T) {
	p := newTestPokemon("Носитель", 100)
	p.CurrentHP = 20
	p.AbilityKey = "blaze"
	p.AbilityName = "Пламя жизни"
	var log []BattleLogEntry
	move := &BattleMove{TypeID: 2}
	if m := abilityModifyOutgoingDamage(&BattleState{}, p, nil, move, 1, &log); m != 1.5 {
		t.Fatalf("blaze mult=%v", m)
	}
	abilityModifyOutgoingDamage(&BattleState{}, p, nil, move, 2, &log)
	if len(log) != 1 {
		t.Fatalf("пассивный лог должен быть один раз за бой, got %d", len(log))
	}
	if !strings.HasPrefix(log[0].Message, "[способность] Пламя жизни: ") {
		t.Fatalf("неверный формат лога: %q", log[0].Message)
	}

	// Триггерные эффекты (лечение) пишутся каждый раз.
	q := newTestPokemon("Целитель", 100)
	q.AbilityKey = "rain_dish"
	q.AbilityName = "Дождефаг"
	q.CurrentHP = 50
	var log2 []BattleLogEntry
	rain := &BattleState{BattleWeatherID: 3}
	abilityOnEndTurn(rain, q, 1, &log2)
	abilityOnEndTurn(rain, q, 2, &log2)
	if len(log2) != 2 {
		t.Fatalf("триггерный лог должен писаться каждый ход, got %d", len(log2))
	}
}

func TestBattleFormSwitchInAndEndTurn(t *testing.T) {
	if _, err := db.DB.Exec(`INSERT INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, primary_type_id, hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult)
		VALUES (9999, 'school', 'schooling', 'Стайная', 3, 25, 1, 20, 2, 1, 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	p := newTestPokemon("Базовая", 100)
	p.PokemonID = 9999
	p.Level = 50
	p.AbilityKey = "schooling"
	bs := &BattleState{}
	var log []BattleLogEntry
	abilityOnSwitchIn(bs, p, &log)
	if p.FormKey != "school" || p.Name != "Стайная" || p.Attack != 200 {
		t.Fatalf("форма на входе не применилась: key=%q name=%q atk=%d", p.FormKey, p.Name, p.Attack)
	}
	p.CurrentHP = 20 // ниже 25% -> базовая форма
	abilityOnEndTurn(bs, p, 2, &log)
	if p.FormKey != "" || p.Name != "Базовая" || p.Attack != 100 {
		t.Fatalf("форма не откатилась в конце хода: key=%q name=%q atk=%d", p.FormKey, p.Name, p.Attack)
	}
}

func TestStanceChangeFormByMove(t *testing.T) {
	if _, err := db.DB.Exec(`INSERT INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, primary_type_id,
		 hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order)
		VALUES (6810, 'shield', 'stance_change', 'Иджислаш', 'Защитная форма', 17,
		 50, 1, 0, 1, 1, 1, 1, 1, 'move', 404, '', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, primary_type_id,
		 hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order)
		VALUES (6810, 'blade', 'stance_change', 'Иджислаш (Атакующая форма)', 'Атакующая форма', 17,
		 50, 1, 0, 3, 0.357142857, 1, 3, 0.357142857, 'move', 0, 'damaging', 0, 1)`); err != nil {
		t.Fatal(err)
	}
	p := newTestPokemon("Иджислаш", 100)
	p.PokemonID = 6810
	p.AbilityKey = "stance_change"
	p.Attack, p.Defense = 50, 140
	p.SpecialAttack, p.SpecialDefense = 50, 140
	bs := &BattleState{}
	var log []BattleLogEntry

	abilityOnSwitchIn(bs, p, &log)
	if p.FormKey != "" {
		t.Fatalf("на входе не должно быть боевой формы: %q", p.FormKey)
	}

	abilityOnMoveUsed(bs, p, &BattleMove{ID: 1, Name: "Атака", CategoryID: 1, Power: 50}, 1, &log)
	if p.FormKey != "blade" || p.Name != "Иджислаш (Атакующая форма)" || p.Attack != 150 || p.Defense != 50 {
		t.Fatalf("атака не включила blade: key=%q name=%q atk=%d def=%d", p.FormKey, p.Name, p.Attack, p.Defense)
	}

	abilityOnEndTurn(bs, p, 2, &log)
	if p.FormKey != "blade" {
		t.Fatalf("в конце хода форма не должна сбрасываться: %q", p.FormKey)
	}

	abilityOnMoveUsed(bs, p, &BattleMove{ID: 425, Name: "Боевой танец", CategoryID: 3}, 3, &log)
	if p.FormKey != "blade" {
		t.Fatalf("статусный ход не должен менять форму: %q", p.FormKey)
	}

	abilityOnMoveUsed(bs, p, &BattleMove{ID: 404, Name: "Королевский щит", CategoryID: 3}, 4, &log)
	if p.FormKey != "" || p.Name != "Иджислаш" || p.Attack != 50 || p.Defense != 140 {
		t.Fatalf("Королевский щит не вернул базовую форму: key=%q name=%q atk=%d def=%d", p.FormKey, p.Name, p.Attack, p.Defense)
	}
}

func TestStanceChangeIntegratesWithMoveHooks(t *testing.T) {
	if _, err := db.DB.Exec(`INSERT INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, primary_type_id,
		 hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order)
		VALUES (6830, 'shield', 'stance_change', 'Иджислаш', 'Защитная форма', 17,
		 50, 1, 0, 1, 1, 1, 1, 1, 'move', 404, '', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO pokemon_forms
		(pokemon_id, form_key, trigger_key, form_name, form_label, primary_type_id,
		 hp_threshold_percent, hp_mode, min_level,
		 attack_mult, defense_mult, speed_mult, special_attack_mult, special_defense_mult,
		 trigger_mode, trigger_move_id, trigger_move_kind, is_base_form, form_order)
		VALUES (6830, 'blade', 'stance_change', 'Иджислаш (Атакующая форма)', 'Атакующая форма', 17,
		 50, 1, 0, 3, 0.357142857, 1, 3, 0.357142857, 'move', 0, 'damaging', 0, 1)`); err != nil {
		t.Fatal(err)
	}
	bs := &BattleState{}
	atk := newTestPokemon("Иджислаш", 100)
	atk.PokemonID = 6830
	atk.AbilityKey = "stance_change"
	atk.Attack, atk.Defense = 50, 140
	atk.SpecialAttack, atk.SpecialDefense = 50, 140
	def := newTestPokemon("Цель", 100)
	var log []BattleLogEntry

	executeDamagingMove(bs, atk, def, &BattleMove{ID: 1, Name: "Атака", CategoryID: 1, Power: 50, Accuracy: 100}, 0, 1, &log, &SideState{}, &SideState{}, false)
	if atk.FormKey != "blade" || atk.Attack != 150 {
		t.Fatalf("executeDamagingMove не включил атакующую форму: key=%q atk=%d", atk.FormKey, atk.Attack)
	}

	handleCustomStatusMove(bs, atk, def, &BattleMove{ID: 404, Name: "Королевский щит", CategoryID: 3}, 2, &log)
	if atk.FormKey != "" || atk.Name != "Иджислаш" || atk.Attack != 50 {
		t.Fatalf("handleCustomStatusMove не вернул защитную форму: key=%q name=%q atk=%d", atk.FormKey, atk.Name, atk.Attack)
	}
}
