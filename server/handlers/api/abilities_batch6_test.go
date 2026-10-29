package api

import (
	"strings"
	"testing"
)

// Точечные проверки механик батча 6 (20 способностей). Без БД: эффекты
// детерминированы, calculateDamageCore зовётся в deterministic-режиме.

func hasSubMsg(log []BattleLogEntry, sub string) bool {
	for _, e := range log {
		if strings.Contains(e.Message, sub) {
			return true
		}
	}
	return false
}

func TestBatch6WeatherSpeed(t *testing.T) {
	bs := &BattleState{BattleWeatherID: 3, PlayerTeam: []BattlePokemon{*newTestPokemon("Пловец", 100)}}
	p := &bs.PlayerTeam[0]
	p.AbilityKey = "swift_swim"
	if got := tailwindSpeed(bs, p); got != 200 {
		t.Fatalf("swift_swim в дождь: speed=%d, want 200", got)
	}
	bs.BattleWeatherID = 2
	p.AbilityKey = "chlorophyll"
	if got := tailwindSpeed(bs, p); got != 200 {
		t.Fatalf("chlorophyll в солнце: speed=%d, want 200", got)
	}
	bs.BattleWeatherID = 1
	if got := tailwindSpeed(bs, p); got != 100 {
		t.Fatalf("chlorophyll вне солнца: speed=%d, want 100", got)
	}
}

func TestBatch6Sturdy(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Здоровяк", 100)
	def.AbilityKey = "sturdy"
	move := &BattleMove{ID: 1, Name: "Сокрушение", TypeID: 1, CategoryID: 1, Power: 999, Accuracy: 100}
	var log []BattleLogEntry
	if _, _ = executeDamagingMove(bs, atk, def, move, 1, 1, &log, &SideState{}, &SideState{}, false); def.CurrentHP != 1 {
		t.Fatalf("sturdy: HP=%d, want 1", def.CurrentHP)
	}
	if !hasSubMsg(log, "1 HP") {
		t.Fatal("sturdy без записи в лог")
	}
	// Второй удар уже не от полного HP — способность не спасает.
	def.CurrentHP = 1
	executeDamagingMove(bs, atk, def, move, 1, 2, &log, &SideState{}, &SideState{}, false)
	if !def.IsFainted {
		t.Fatalf("sturdy не должен спасать не с полного HP: HP=%d", def.CurrentHP)
	}
	// OHKO-атаки блокируются.
	def2 := newTestPokemon("Здоровяк2", 100)
	def2.AbilityKey = "sturdy"
	ohko := &BattleMove{ID: 462, Name: "Гильотина", TypeID: 1, CategoryID: 1, Power: 0, Accuracy: 30}
	if checkHit(&BattleState{}, ohko, atk, def2, &log) {
		t.Fatal("sturdy должен блокировать OHKO-атаку")
	}
}

func TestBatch6InnerFocus(t *testing.T) {
	p := newTestPokemon("Бесстрашный", 100)
	p.AbilityKey = "inner_focus"
	var log []BattleLogEntry
	inflictFlinch(p, 1, &log)
	if p.Flinched {
		t.Fatal("inner_focus: испуг не должен накладываться")
	}
	if !hasSubMsg(log, "не поддаётся испугу") {
		t.Fatal("inner_focus без записи в лог")
	}
}

func TestBatch6SheerForce(t *testing.T) {
	move := &BattleMove{ID: 7, Name: "Огненный кулак", TypeID: 2, CategoryID: 1, Power: 80, Accuracy: 100, EffectID: 3, EffectChance: 10}
	if !abilitySheerForceApplies(move) {
		t.Fatal("sheer_force должен видеть вторичный эффект")
	}
	plain := &BattleMove{ID: 7, Name: "Огненный кулак", TypeID: 2, CategoryID: 1, Power: 80, Accuracy: 100}
	if abilitySheerForceApplies(plain) {
		t.Fatal("sheer_force не должен срабатывать без вторичного эффекта")
	}
	guaranteed := &BattleMove{ID: 8, Name: "Инферно", CategoryID: 1, Power: 100, EffectID: 3, EffectChance: 100}
	if !abilitySheerForceApplies(guaranteed) {
		t.Fatal("sheer_force должен срабатывать на гарантированный вторичный эффект")
	}
	selfBuff := &BattleMove{ID: 9, Name: "Суперсила", CategoryID: 1, Power: 120,
		StatChanges: []StatChange{{StatID: 1, Stages: -1, Target: 1, Chance: 100}}}
	if abilitySheerForceApplies(selfBuff) {
		t.Fatal("sheer_force не должен трогать гарантированные само-изменения")
	}
	suppressed := abilitySuppressSecondaryMove(move)
	if suppressed.EffectID != 0 || suppressed.EffectChance != 0 {
		t.Fatalf("sheer_force не подавил эффект: id=%d chance=%d", suppressed.EffectID, suppressed.EffectChance)
	}
	def := newTestPokemon("Цель", 100)
	atk1 := newTestPokemon("Обычный", 100)
	atk2 := newTestPokemon("Грубый", 100)
	atk2.AbilityKey = "sheer_force"
	var log []BattleLogEntry
	d1, _, _ := calculateDamageCore(&BattleState{}, atk1, def, move, &SideState{}, 1, &log, true)
	d2, _, _ := calculateDamageCore(&BattleState{}, atk2, def, move, &SideState{}, 1, &log, true)
	if d2 <= d1 {
		t.Fatalf("sheer_force не усилил урон: %d <= %d", d2, d1)
	}
}

func TestBatch6Pressure(t *testing.T) {
	move := &BattleMove{PP: 10}
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Величие", 100)
	def.AbilityKey = "pressure"
	if !abilityPressureDrain(atk, def, move) || move.PP != 9 {
		t.Fatalf("pressure: PP=%d, want 9", move.PP)
	}
	def.AbilityKey = ""
	move.PP = 10
	if abilityPressureDrain(atk, def, move) || move.PP != 10 {
		t.Fatalf("без pressure PP не должен меняться: %d", move.PP)
	}
}

func TestBatch6OwnTempo(t *testing.T) {
	p := newTestPokemon("Ритмичный", 100)
	p.AbilityKey = "own_tempo"
	var log []BattleLogEntry
	inflictConfusion(p, 1, &log)
	if p.Confused {
		t.Fatal("own_tempo: замешательство не должно накладываться")
	}
	if !hasSubMsg(log, "замешательство не действует") {
		t.Fatal("own_tempo без записи в лог")
	}
}

func TestBatch6Oblivious(t *testing.T) {
	p := newTestPokemon("Забывчивый", 100)
	p.AbilityKey = "oblivious"
	var log []BattleLogEntry
	inflictInfatuation(p, 1, &log)
	if p.Infatuated {
		t.Fatal("oblivious: влюблённость не должна накладываться")
	}
	atk := newTestPokemon("Насмешник", 100)
	taunt := &BattleMove{ID: moveTaunt, Name: "Насмешка", CategoryID: 3}
	handleBlockMove(atk, p, taunt, 1, &log)
	if p.TauntTurns != 0 {
		t.Fatalf("oblivious: Насмешка не должна действовать, turns=%d", p.TauntTurns)
	}
}

func TestBatch6Infiltrator(t *testing.T) {
	atk := newTestPokemon("Барьерный", 100)
	def := newTestPokemon("Защитник", 100)
	move := &BattleMove{ID: 1, Name: "Удар", TypeID: 1, CategoryID: 1, Power: 80, Accuracy: 100}
	side := &SideState{ReflectTurns: 5}
	var log []BattleLogEntry
	plain, _, _ := calculateDamageCore(&BattleState{}, atk, def, move, side, 1, &log, true)
	atk.AbilityKey = "infiltrator"
	through, _, _ := calculateDamageCore(&BattleState{}, atk, def, move, side, 1, &log, true)
	if through <= plain {
		t.Fatalf("infiltrator не прошёл экран: %d <= %d", through, plain)
	}
	// Ограждение тоже игнорируется.
	def.StatusID = 0
	statusMove := &BattleMove{ID: 2, Name: "Паралич", TypeID: 1, CategoryID: 3, TargetID: 1, EffectID: 2, EffectChance: 100}
	applyEffectByID(&BattleState{}, atk, def, statusMove, 1, &log, &SideState{SafeguardTurns: 5})
	if def.StatusID != 2 {
		t.Fatalf("infiltrator не прошёл Ограждение: status=%d", def.StatusID)
	}
	def.StatusID = 0
	atk.AbilityKey = ""
	applyEffectByID(&BattleState{}, atk, def, statusMove, 1, &log, &SideState{SafeguardTurns: 5})
	if def.StatusID != 0 {
		t.Fatalf("без infiltrator Ограждение должно блокировать: status=%d", def.StatusID)
	}
}

func TestBatch6TelepathyFrisk(t *testing.T) {
	bs := &BattleState{PlayerTeam: []BattlePokemon{*newTestPokemon("Читающий", 100)}}
	bs.ActiveIdx = 0
	opp := newTestPokemon("Противник", 100)
	opp.HeldItemName = "Остатки"
	opp.Moves = []BattleMove{
		{Name: "Гипноз", CategoryID: 3},
		{Name: "Удар", CategoryID: 1},
	}
	bs.WildPokemon = opp
	p := &bs.PlayerTeam[0]
	p.AbilityKey = "frisk"
	var log []BattleLogEntry
	abilityOnSwitchIn(bs, p, &log)
	if !hasSubMsg(log, "Остатки") {
		t.Fatal("frisk не определил предмет")
	}
	log = nil
	p.AbilityKey = "telepathy"
	abilityOnSwitchIn(bs, p, &log)
	if !hasSubMsg(log, "Гипноз") {
		t.Fatal("telepathy не показал статусные атаки")
	}
}

func TestBatch6Guts(t *testing.T) {
	def := newTestPokemon("Цель", 100)
	move := &BattleMove{ID: 1, Name: "Удар", TypeID: 1, CategoryID: 1, Power: 80, Accuracy: 100}
	plain := newTestPokemon("Обычный", 100)
	plain.StatusID = 3
	guts := newTestPokemon("Болевой", 100)
	guts.StatusID = 3
	guts.AbilityKey = "guts"
	var log []BattleLogEntry
	d1, _, _ := calculateDamageCore(&BattleState{}, plain, def, move, &SideState{}, 1, &log, true)
	d2, _, _ := calculateDamageCore(&BattleState{}, guts, def, move, &SideState{}, 1, &log, true)
	if d2 <= d1 {
		t.Fatalf("guts не усилил атаку при ожоге: %d <= %d", d2, d1)
	}
}

func TestBatch6Hydration(t *testing.T) {
	p := newTestPokemon("Впитывающий", 100)
	p.AbilityKey = "hydration"
	p.StatusID = 6
	bs := &BattleState{BattleWeatherID: 3}
	var log []BattleLogEntry
	abilityOnEndTurn(bs, p, 1, &log)
	if p.StatusID != 1 {
		t.Fatalf("hydration не снял статус в дождь: status=%d", p.StatusID)
	}
	p.StatusID = 6
	bs.BattleWeatherID = 2
	abilityOnEndTurn(bs, p, 2, &log)
	if p.StatusID != 6 {
		t.Fatalf("hydration не должен работать вне дождя: status=%d", p.StatusID)
	}
}

func TestBatch6Regenerator(t *testing.T) {
	p := newTestPokemon("Регенератор", 90)
	p.AbilityKey = "regenerator"
	p.CurrentHP = 30
	var log []BattleLogEntry
	abilityOnSwitchOut(p, 1, &log)
	if p.CurrentHP != 60 {
		t.Fatalf("regenerator: HP=%d, want 60", p.CurrentHP)
	}
	p.IsFainted = true
	p.CurrentHP = 0
	abilityOnSwitchOut(p, 2, &log)
	if p.CurrentHP != 0 {
		t.Fatalf("regenerator не должен лечить павшего: HP=%d", p.CurrentHP)
	}
}

func TestBatch6Overcoat(t *testing.T) {
	p := newTestPokemon("Укрытие", 160)
	p.AbilityKey = "overcoat"
	var log []BattleLogEntry
	weatherChipOne(weatherSand, 1, p, &log)
	if p.CurrentHP != 160 {
		t.Fatalf("overcoat не защитил от песчаной бури: HP=%d", p.CurrentHP)
	}
	plain := newTestPokemon("Обычный", 160)
	weatherChipOne(weatherSand, 1, plain, &log)
	if plain.CurrentHP >= 160 {
		t.Fatalf("обычный покемон должен страдать от бури: HP=%d", plain.CurrentHP)
	}
	powder := &BattleMove{ID: 393, Name: "Споры", CategoryID: 3, TargetID: 1}
	if !abilityAbsorbsStatusMove(&BattleState{}, newTestPokemon("Атакующий", 100), p, powder, 1, &log) {
		t.Fatal("overcoat должен блокировать порошковые ходы")
	}
	applyPowderMove(p, 1, &log)
	if p.Powdered {
		t.Fatal("overcoat не должен покрываться порошком")
	}
}

func TestBatch6Hustle(t *testing.T) {
	atk := newTestPokemon("Нетерпеливый", 100)
	atk.AbilityKey = "hustle"
	def := newTestPokemon("Цель", 100)
	move := &BattleMove{ID: 1, Name: "Удар", TypeID: 1, CategoryID: 1, Power: 80, Accuracy: 100}
	if got := abilityAccuracyMult(atk, def, move); got != 0.8 {
		t.Fatalf("hustle accuracy=%v, want 0.8", got)
	}
	hustle := newTestPokemon("Нетерпеливый", 100)
	hustle.AbilityKey = "hustle"
	plain := newTestPokemon("Обычный", 100)
	var log []BattleLogEntry
	d1, _, _ := calculateDamageCore(&BattleState{}, plain, def, move, &SideState{}, 1, &log, true)
	d2, _, _ := calculateDamageCore(&BattleState{}, hustle, def, move, &SideState{}, 1, &log, true)
	if d2 <= d1 {
		t.Fatalf("hustle не усилил физическую атаку: %d <= %d", d2, d1)
	}
}

func TestBatch6LeafGuard(t *testing.T) {
	p := newTestPokemon("Лиственный", 100)
	p.AbilityKey = "leaf_guard"
	if !abilityBlocksStatus(&BattleState{BattleWeatherID: 2}, p, 6) {
		t.Fatal("leaf_guard должен блокировать статус в солнце")
	}
	if abilityBlocksStatus(&BattleState{BattleWeatherID: 3}, p, 6) {
		t.Fatal("leaf_guard не должен работать вне солнца")
	}
}

func TestBatch6Technician(t *testing.T) {
	def := newTestPokemon("Цель", 100)
	weak := &BattleMove{ID: 1, Name: "Слабый", TypeID: 1, CategoryID: 1, Power: 60, Accuracy: 100}
	strong := &BattleMove{ID: 2, Name: "Сильный", TypeID: 1, CategoryID: 1, Power: 61, Accuracy: 100}
	plain := newTestPokemon("Обычный", 100)
	tech := newTestPokemon("Мастер", 100)
	tech.AbilityKey = "technician"
	var log []BattleLogEntry
	d1, _, _ := calculateDamageCore(&BattleState{}, plain, def, weak, &SideState{}, 1, &log, true)
	d2, _, _ := calculateDamageCore(&BattleState{}, tech, def, weak, &SideState{}, 1, &log, true)
	if d2 <= d1 {
		t.Fatalf("technician не усилил слабую атаку: %d <= %d", d2, d1)
	}
	d3, _, _ := calculateDamageCore(&BattleState{}, plain, def, strong, &SideState{}, 1, &log, true)
	d4, _, _ := calculateDamageCore(&BattleState{}, tech, def, strong, &SideState{}, 1, &log, true)
	if d4 != d3 {
		t.Fatalf("technician усилил атаку >60: %d != %d", d4, d3)
	}
}

func TestBatch6MoldBreaker(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Подавляющий", 100)
	atk.AbilityKey = "mold_breaker"
	def := newTestPokemon("Аккумулятор", 100)
	def.AbilityKey = "volt_absorb"
	electric := &BattleMove{ID: 1, Name: "Разряд", TypeID: 4, CategoryID: 1, Power: 80, Accuracy: 100}
	var log []BattleLogEntry
	if abilityAbsorbsMove(bs, atk, def, electric, 1, &log) {
		t.Fatal("mold_breaker должен игнорировать поглощение")
	}
	def.AbilityKey = "thick_fat"
	if mult := abilityModifyIncomingDamage(bs, atk, def, &BattleMove{TypeID: 2, CategoryID: 2}, 1, &log); mult != 1.0 {
		t.Fatalf("mold_breaker должен игнорировать снижение урона: %v", mult)
	}
	// Стойкость тоже подавляется.
	def = newTestPokemon("Здоровяк", 100)
	def.AbilityKey = "sturdy"
	executeDamagingMove(bs, atk, def, &BattleMove{ID: 2, Name: "Сокрушение", TypeID: 1, CategoryID: 1, Power: 999, Accuracy: 100}, 1, 2, &log, &SideState{}, &SideState{}, false)
	if !def.IsFainted {
		t.Fatalf("mold_breaker должен обходить sturdy: HP=%d", def.CurrentHP)
	}
}

func TestBatch6Damp(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Взрывающийся", 100)
	def := newTestPokemon("Сырой", 100)
	def.AbilityKey = "damp"
	move := &BattleMove{ID: 570, Name: "Самоподрыв", TypeID: 1, CategoryID: 1, Power: 200, Accuracy: 100}
	var log []BattleLogEntry
	executeDamagingMove(bs, atk, def, move, 1, 1, &log, &SideState{}, &SideState{}, false)
	if atk.IsFainted || atk.CurrentHP != 100 {
		t.Fatalf("damp должен блокировать самоподрыв: HP=%d fainted=%v", atk.CurrentHP, atk.IsFainted)
	}
	if !hasSubMsg(log, "гасит саморазрушающую атаку") {
		t.Fatal("damp без записи в лог")
	}
}
