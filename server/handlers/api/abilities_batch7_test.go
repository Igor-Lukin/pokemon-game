package api

import (
	"testing"
)

// Групповые проверки расширенного батча 7+. Без БД, детерминированные ветки.

func TestBatch7TypeAbsorbs(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Носитель", 100)
	var log []BattleLogEntry

	def.AbilityKey = "lightning_rod"
	if !abilityAbsorbsMoveExtra(bs, atk, def, &BattleMove{TypeID: typeElectric, CategoryID: 2}, 1, &log) || def.StatStages[4] != 1 {
		t.Fatalf("lightning_rod: absorb=%v spa=%d", def.StatStages[4], def.StatStages[4])
	}
	def = newTestPokemon("Носитель", 100)
	def.AbilityKey = "water_defense"
	if !abilityAbsorbsMoveExtra(bs, atk, def, &BattleMove{TypeID: typeWater, CategoryID: 1}, 1, &log) || def.StatStages[2] != 2 {
		t.Fatalf("water_defense: def=%d", def.StatStages[2])
	}
	def = newTestPokemon("Носитель", 100)
	def.AbilityKey = "earth_eater"
	def.CurrentHP = 40
	if !abilityAbsorbsMoveExtra(bs, atk, def, &BattleMove{TypeID: typeGround, CategoryID: 1}, 1, &log) || def.CurrentHP != 65 {
		t.Fatalf("earth_eater: hp=%d", def.CurrentHP)
	}
	def = newTestPokemon("Носитель", 100)
	def.AbilityKey = "well_baked_body"
	if !abilityAbsorbsMoveExtra(bs, atk, def, &BattleMove{TypeID: typeFire, CategoryID: 1}, 1, &log) || def.StatStages[2] != 2 {
		t.Fatalf("well_baked_body: def=%d", def.StatStages[2])
	}
	def = newTestPokemon("Носитель", 100)
	def.AbilityKey = "bulletproof"
	if !abilityAbsorbsMoveExtra(bs, atk, def, &BattleMove{EngName: "Shadow Ball", TypeID: typeGhost, CategoryID: 2}, 1, &log) {
		t.Fatal("bulletproof не заблокировал снаряд")
	}
	def = newTestPokemon("Носитель", 100)
	def.AbilityKey = "soundproof"
	if !abilityAbsorbsMoveExtra(bs, atk, def, &BattleMove{EngName: "Hyper Voice", Sound: true, CategoryID: 2}, 1, &log) {
		t.Fatal("soundproof не заблокировал звук")
	}
	def = newTestPokemon("Носитель", 100)
	def.AbilityKey = "wind_rider"
	if !abilityAbsorbsMoveExtra(bs, atk, def, &BattleMove{EngName: "Air Cutter", CategoryID: 2}, 1, &log) || def.StatStages[1] != 1 {
		t.Fatalf("wind_rider: atk=%d", def.StatStages[1])
	}
}

func TestBatch7IncomingDamage(t *testing.T) {
	atk := newTestPokemon("Атакующий", 100)
	moveSpec := &BattleMove{ID: 1, EngName: "Psychic", TypeID: typePsychic, CategoryID: 2, Power: 80, Accuracy: 100}
	plain := newTestPokemon("Обычный", 100)
	ms := newTestPokemon("Многослойный", 100)
	ms.AbilityKey = "multiscale"
	var log []BattleLogEntry
	d1, _, _ := calculateDamageCore(&BattleState{}, atk, plain, moveSpec, &SideState{}, 1, &log, true)
	d2, _, _ := calculateDamageCore(&BattleState{}, atk, ms, moveSpec, &SideState{}, 1, &log, true)
	if d2 >= d1 {
		t.Fatalf("multiscale не снизил урон: %d >= %d", d2, d1)
	}
	ice := newTestPokemon("Ледяная чешуя", 100)
	ice.AbilityKey = "ice_scales"
	d3, _, _ := calculateDamageCore(&BattleState{}, atk, ice, moveSpec, &SideState{}, 1, &log, true)
	if d3 >= d1 {
		t.Fatalf("ice_scales не снизил спец. урон: %d >= %d", d3, d1)
	}
	fire := &BattleMove{ID: 2, TypeID: typeFire, CategoryID: 2, Power: 80, Accuracy: 100}
	heat := newTestPokemon("Огнеупорный", 100)
	heat.AbilityKey = "heatproof"
	d4, _, _ := calculateDamageCore(&BattleState{}, atk, heat, fire, &SideState{}, 1, &log, true)
	d5, _, _ := calculateDamageCore(&BattleState{}, atk, plain, fire, &SideState{}, 1, &log, true)
	if d4 >= d5 {
		t.Fatalf("heatproof не снизил огонь: %d >= %d", d4, d5)
	}
}

func TestBatch7StatReactions(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Защитник", 100)
	var log []BattleLogEntry
	selfUp := &BattleMove{CategoryID: 3, StatChanges: []StatChange{{StatID: 1, Stages: 1, Target: 1, Chance: 100}}}
	simple := newTestPokemon("Простой", 100)
	simple.AbilityKey = "simple"
	applyStatChanges(simple, def, selfUp, 1, 1, &log, &SideState{})
	if simple.StatStages[1] != 2 {
		t.Fatalf("simple: atk stage=%d, want 2", simple.StatStages[1])
	}
	contrary := newTestPokemon("Противоположный", 100)
	contrary.AbilityKey = "contrary"
	down := &BattleMove{CategoryID: 3, StatChanges: []StatChange{{StatID: 2, Stages: -1, Target: 1, Chance: 100}}}
	applyStatChanges(contrary, def, down, 1, 1, &log, &SideState{})
	if contrary.StatStages[2] != 1 {
		t.Fatalf("contrary: def stage=%d, want 1", contrary.StatStages[2])
	}
	defiant := newTestPokemon("Дерзкий", 100)
	defiant.AbilityKey = "defiant"
	targetDown := &BattleMove{CategoryID: 3, StatChanges: []StatChange{{StatID: 2, Stages: -1, Target: 2, Chance: 100}}}
	applyStatChanges(atk, defiant, targetDown, 1, 1, &log, &SideState{})
	if defiant.StatStages[1] != 2 {
		t.Fatalf("defiant: atk stage=%d, want 2", defiant.StatStages[1])
	}
	mirror := newTestPokemon("Зеркальный", 100)
	mirror.AbilityKey = "mirror_armor"
	applyStatChanges(atk, mirror, targetDown, 1, 1, &log, &SideState{})
	if atk.StatStages[2] != -1 || mirror.StatStages[2] != 0 {
		t.Fatalf("mirror_armor: atk=%d mirror=%d", atk.StatStages[2], mirror.StatStages[2])
	}
	side := &SideState{FlowerVeilTurns: 5}
	veiled := newTestPokemon("Цветочный", 100)
	applyStatChanges(atk, veiled, targetDown, 1, 1, &log, side)
	if veiled.StatStages[2] != 0 {
		t.Fatalf("flower_veil: def=%d, want 0", veiled.StatStages[2])
	}
	_ = bs
}

func TestBatch7PrioritySpeed(t *testing.T) {
	bs := &BattleState{BattleWeatherID: weatherSand, PlayerTeam: []BattlePokemon{*newTestPokemon("Песчаный", 100)}}
	p := &bs.PlayerTeam[0]
	p.AbilityKey = "sand_rush"
	if got := tailwindSpeed(bs, p); got != 200 {
		t.Fatalf("sand_rush speed=%d", got)
	}
	bs.BattleWeatherID = weatherHail
	p.AbilityKey = "slush_rush"
	if got := tailwindSpeed(bs, p); got != 200 {
		t.Fatalf("slush_rush speed=%d", got)
	}
	p.AbilityKey = "prankster"
	if pr := abilityMovePriority(bs, p, &BattleMove{CategoryID: 3}); pr != 1 {
		t.Fatalf("prankster priority=%d", pr)
	}
	p.AbilityKey = "gale_wings"
	p.CurrentHP = p.MaxHP
	if pr := abilityMovePriority(bs, p, &BattleMove{TypeID: typeFlying}); pr != 1 {
		t.Fatalf("gale_wings priority=%d", pr)
	}
	p.AbilityKey = "triage"
	if pr := abilityMovePriority(bs, p, &BattleMove{HealPercent: 50}); pr != 3 {
		t.Fatalf("triage priority=%d", pr)
	}
	p.AbilityKey = "stall"
	if pr := abilityMovePriority(bs, p, &BattleMove{}); pr != -10 {
		t.Fatalf("stall priority=%d", pr)
	}
	def := newTestPokemon("Королевский", 100)
	def.AbilityKey = "queenly_majesty"
	if !abilityBlocksPriorityMove(def, &BattleMove{Priority: 1}) {
		t.Fatal("queenly_majesty не заблокировал приоритет")
	}
}

func TestBatch7AccuracyCritHits(t *testing.T) {
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Цель", 100)
	atk.AbilityKey = "no_guard"
	if !checkHit(&BattleState{}, &BattleMove{Accuracy: 1}, atk, def, &[]BattleLogEntry{}) {
		t.Fatal("no_guard не дал попасть")
	}
	def.AbilityKey = "wonder_skin"
	if got := abilityAccuracyMult(atk, def, &BattleMove{CategoryID: 3}); got != 0.5 {
		t.Fatalf("wonder_skin mult=%v", got)
	}
	if abilityCritStageExtra(&BattlePokemon{AbilityKey: "super_luck"}) != 1 {
		t.Fatal("super_luck не дал +1 крит-стадию")
	}
	if !abilityAlwaysCrit(&BattlePokemon{AbilityKey: "mercy"}) {
		t.Fatal("mercy не всегда крит")
	}
	if !abilityMaxHits(&BattlePokemon{AbilityKey: "skill_link"}) {
		t.Fatal("skill_link не максимизирует удары")
	}
}

func TestBatch7SwitchIn(t *testing.T) {
	base := func() (*BattleState, *BattlePokemon, *BattlePokemon) {
		bs := &BattleState{PlayerTeam: []BattlePokemon{*newTestPokemon("Игрок", 100)}}
		bs.ActiveIdx = 0
		opp := newTestPokemon("Враг", 100)
		opp.Defense, opp.SpecialDefense = 80, 120
		opp.AbilityKey = "static"
		bs.WildPokemon = opp
		return bs, &bs.PlayerTeam[0], opp
	}
	var log []BattleLogEntry

	bs, p, _ := base()
	p.AbilityKey = "download"
	abilityExtraSwitchIn(bs, p, &log)
	if p.StatStages[1] != 1 {
		t.Fatalf("download: atk=%d, want 1", p.StatStages[1])
	}
	bs, p, _ = base()
	p.AbilityKey = "intrepid_sword"
	abilityExtraSwitchIn(bs, p, &log)
	if p.StatStages[1] != 1 {
		t.Fatalf("intrepid_sword: atk=%d", p.StatStages[1])
	}
	bs, p, _ = base()
	p.AbilityKey = "dauntless_shield"
	abilityExtraSwitchIn(bs, p, &log)
	if p.StatStages[2] != 1 {
		t.Fatalf("dauntless_shield: def=%d", p.StatStages[2])
	}
	bs, p, opp := base()
	p.AbilityKey = "guard_dog"
	abilityExtraSwitchIn(bs, p, &log)
	if opp.StatStages[1] != -1 || !opp.NoEscape {
		t.Fatalf("guard_dog: atk=%d no_escape=%v", opp.StatStages[1], opp.NoEscape)
	}
	bs, p, _ = base()
	p.AbilityKey = "sand_stream"
	abilityExtraSwitchIn(bs, p, &log)
	if bs.BattleWeatherID != weatherSand {
		t.Fatalf("sand_stream weather=%d", bs.BattleWeatherID)
	}
	bs, p, _ = base()
	p.AbilityKey = "misty_surge"
	abilityExtraSwitchIn(bs, p, &log)
	if bs.TerrainID != terrainMisty {
		t.Fatalf("misty_surge terrain=%d", bs.TerrainID)
	}
	bs, p, opp = base()
	bs.PlayerSide.ReflectTurns = 5
	bs.WildSide.LightScreenTurns = 5
	p.AbilityKey = "screen_cleaner"
	abilityExtraSwitchIn(bs, p, &log)
	if bs.PlayerSide.ReflectTurns != 0 || bs.WildSide.LightScreenTurns != 0 {
		t.Fatal("screen_cleaner не очистил экраны")
	}
	bs, p, opp = base()
	p.AbilityKey = "sweet_syrup"
	abilityExtraSwitchIn(bs, p, &log)
	if opp.StatStages[3] != -1 {
		t.Fatalf("sweet_syrup: speed=%d", opp.StatStages[3])
	}
	bs, p, opp = base()
	p.AbilityKey = "trace"
	abilityExtraSwitchIn(bs, p, &log)
	if p.AbilityKey != "static" {
		t.Fatalf("trace: ability=%q", p.AbilityKey)
	}
	bs, p, _ = base()
	p.AbilityKey = "armor_tail"
	abilityExtraSwitchIn(bs, p, &log)
	if bs.PlayerSide.LightScreenTurns != 5 {
		t.Fatalf("armor_tail: screen=%d", bs.PlayerSide.LightScreenTurns)
	}
}

func TestBatch7ContactAndKO(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Атакующий", 100)
	def := newTestPokemon("Носитель", 100)
	var log []BattleLogEntry

	def.AbilityKey = "stamina"
	abilityExtraAfterHit(bs, atk, def, &BattleMove{CategoryID: 1}, 1, &log)
	if def.StatStages[2] != 1 {
		t.Fatalf("stamina: def=%d", def.StatStages[2])
	}
	def.AbilityKey = "mummy"
	abilityExtraContact(bs, atk, def, &BattleMove{Contact: true}, 1, &log)
	if atk.AbilityKey != "mummy" {
		t.Fatalf("mummy: atk ability=%q", atk.AbilityKey)
	}
	atk = newTestPokemon("Вор", 100)
	def = newTestPokemon("Карманник", 100)
	def.AbilityKey = "pickpocket"
	atk.HeldItemID, atk.HeldItemName = 5, "Остатки"
	abilityExtraContact(bs, atk, def, &BattleMove{Contact: true}, 1, &log)
	if def.HeldItemID != 5 || atk.HeldItemID != 0 {
		t.Fatalf("pickpocket: def=%d atk=%d", def.HeldItemID, atk.HeldItemID)
	}
	def = newTestPokemon("Гиблое тело", 100)
	def.AbilityKey = "perish_body"
	atk = newTestPokemon("Атакующий", 100)
	abilityExtraContact(bs, atk, def, &BattleMove{Contact: true}, 1, &log)
	if atk.PerishCount != 3 || def.PerishCount != 3 {
		t.Fatalf("perish_body: atk=%d def=%d", atk.PerishCount, def.PerishCount)
	}
	atk = newTestPokemon("Победитель", 100)
	atk.AbilityKey = "moxie"
	def = newTestPokemon("Цель", 100)
	abilityExtraOnKO(bs, atk, def, 1, &log)
	if atk.StatStages[1] != 1 {
		t.Fatalf("moxie: atk=%d", atk.StatStages[1])
	}
	atk = newTestPokemon("Зверь", 100)
	atk.AbilityKey = "beast_boost"
	atk.Attack, atk.Defense, atk.Speed = 50, 150, 60
	abilityExtraOnKO(bs, atk, def, 1, &log)
	if atk.StatStages[2] != 1 {
		t.Fatalf("beast_boost: def stage=%d", atk.StatStages[2])
	}
}

func TestSelfDestructExplosionFaintsUser(t *testing.T) {
	bs := &BattleState{}
	atk := newTestPokemon("Взрывающийся", 100)
	def := newTestPokemon("Цель", 100)
	var log []BattleLogEntry
	executeDamagingMove(bs, atk, def, &BattleMove{ID: 100, Name: "Взрыв", TypeID: typeNormal, CategoryID: 1, Power: 250, Accuracy: 100}, 1, 1, &log, &SideState{}, &SideState{}, false)
	if !atk.IsFainted || atk.CurrentHP != 0 {
		t.Fatalf("Взрыв: пользователь должен пасть в обморок, HP=%d fainted=%v", atk.CurrentHP, atk.IsFainted)
	}
}

func TestBatch7Misc(t *testing.T) {
	bs := &BattleState{}
	var log []BattleLogEntry

	// Liquid Ooze: дренаж вредит атакующему.
	atk := newTestPokemon("Дренаж", 100)
	atk.CurrentHP = 50
	def := newTestPokemon("Гниющая кожа", 100)
	def.AbilityKey = "liquid_ooze"
	abilityLiquidOoze(atk, def, &BattleMove{Drain: 50}, 40, 50, 1, &log)
	if atk.CurrentHP != 30 {
		t.Fatalf("liquid_ooze: hp=%d, want 30", atk.CurrentHP)
	}
	// Magic Guard: погодный урон не проходит.
	mg := newTestPokemon("Магический щит", 100)
	mg.AbilityKey = "magic_guard"
	weatherChipOne(weatherSand, 1, mg, &log)
	if mg.CurrentHP != 100 {
		t.Fatalf("magic_guard: hp=%d", mg.CurrentHP)
	}
	// Shield Dust: вторичный статус не накладывается.
	sd := newTestPokemon("Чешуекрылое", 100)
	sd.AbilityKey = "shield_dust"
	applyEffectByID(bs, atk, sd, &BattleMove{CategoryID: 1, EffectID: 3, EffectChance: 100}, 1, &log, &SideState{})
	if sd.StatusID != 0 {
		t.Fatalf("shield_dust: status=%d", sd.StatusID)
	}
	// Serene Grace.
	if got := abilityEffectChance(&BattlePokemon{AbilityKey: "serene_grace"}, 30); got != 60 {
		t.Fatalf("serene_grace: chance=%d", got)
	}
	// Corrosion обходит иммунитет Стали к яду.
	steel := newTestPokemon("Сталь", 100)
	steel.PrimaryTypeID = typeSteel
	corr := newTestPokemon("Коррозия", 100)
	corr.AbilityKey = "corrosion"
	applyEffectByID(bs, corr, steel, &BattleMove{CategoryID: 3, EffectID: 6, EffectChance: 100}, 1, &log, &SideState{})
	if steel.StatusID != 6 {
		t.Fatalf("corrosion: status=%d, want 6", steel.StatusID)
	}
	// Disguise: первый удар не проходит.
	dg := newTestPokemon("Маскировка", 100)
	dg.AbilityKey = "disguise"
	hit := newTestPokemon("Атакующий", 100)
	dmg, _, _ := calculateDamageCore(bs, hit, dg, &BattleMove{CategoryID: 1, Power: 80}, &SideState{}, 1, &log, true)
	if !abilityBlocksFirstHit(dg, &BattleMove{CategoryID: 1}, 1, &log) {
		t.Fatal("disguise не заблокировал удар")
	}
	if dmg == 0 {
		t.Fatal("calculateDamage не должен меняться (блок в execute)")
	}
	// Aftermath: предсмертная контратака.
	att := newTestPokemon("Убийца", 100)
	aft := newTestPokemon("Отмщение", 100)
	aft.AbilityKey = "aftermath_quarter"
	abilityOnFaintRevenge(att, aft, &BattleMove{Contact: true}, 10, 1, &log)
	if att.CurrentHP != 75 {
		t.Fatalf("aftermath: hp=%d, want 75", att.CurrentHP)
	}
	// Bad Dreams.
	bd := newTestPokemon("Кошмары", 100)
	bd.AbilityKey = "bad_dreams"
	sleeper := newTestPokemon("Спящий", 100)
	sleeper.StatusID = 5
	bs.WildPokemon = sleeper
	abilityExtraEndTurn(bs, bd, 1, &log)
	if sleeper.CurrentHP >= 100 {
		t.Fatalf("bad_dreams: hp=%d", sleeper.CurrentHP)
	}
	// Преобразование атак.
	px := newTestPokemon("Пиксилят", 100)
	px.AbilityKey = "pixilate"
	normalMove := &BattleMove{TypeID: typeNormal, Power: 100}
	converted := abilityAdjustMove(px, normalMove)
	if converted.TypeID != typeFairy || converted.Power != 120 {
		t.Fatalf("pixilate: type=%d power=%d", converted.TypeID, converted.Power)
	}
	lr := newTestPokemon("Длиннорукий", 100)
	lr.AbilityKey = "long_reach"
	if abilityAdjustMove(lr, &BattleMove{Contact: true}).Contact {
		t.Fatal("long_reach не убрал контакт")
	}
	// Early Bird / Coma.
	sleep := newTestPokemon("Жаворонок", 100)
	sleep.StatusID = 5
	sleep.SleepTurns = 3
	sleep.AbilityKey = "early_bird"
	if statusRegistry[5].CanAct(sleep, &log) || sleep.SleepTurns != 1 {
		t.Fatalf("early_bird: turns=%d, want still asleep with 1", sleep.SleepTurns)
	}
	coma := newTestPokemon("Кома", 100)
	coma.StatusID = 5
	coma.AbilityKey = "coma"
	if !statusRegistry[5].CanAct(coma, &log) {
		t.Fatal("coma должен действовать во сне")
	}
}
