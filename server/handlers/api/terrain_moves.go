package api

import "strconv"

// terrain_moves.go — 4 арены (терраина). Общеполевой таймер на BattleState (как
// погода): 5 ходов, ВЗАИМОИСКЛЮЧАЮЩИЕ. По решению владельца эффекты действуют на
// ОБОИХ активных без различия «наземный/летающий».
//   539 Травяная (Grassy)     — Трава ×1.5; лечит обоих 1/16/ход; Землетрясение ×0.5.
//   541 Туманная (Misty)      — Дракон ×0.5; нельзя наложить мажорный статус.
//   548 Электрическая (Electric) — Электро ×1.5; нельзя усыпить.
//   498 Ментальная (Psychic)  — Психика ×1.5; блок приоритетных урон-атак (prio>0).
// Постановка — applySideMove; отсчёт — tickSideTimers; эффекты — calculateDamage
// (множители), executeDamagingMove (блок приоритета), applyEffectByID (блок статусов).

const (
	terrainNone     = 0
	terrainGrassy   = 1
	terrainMisty    = 2
	terrainElectric = 3
	terrainPsychic  = 4
	terrainDuration = 5

	moveGrassyTerrain   = 539
	moveMistyTerrain    = 541
	moveElectricTerrain = 548
	movePsychicTerrain  = 498
	moveEarthquake      = 74 // Землетрясение — ослабляется Травяной ареной
)

// terrainByMove — id хода-сеттера -> id арены.
var terrainByMove = map[int]int{
	moveGrassyTerrain:   terrainGrassy,
	moveMistyTerrain:    terrainMisty,
	moveElectricTerrain: terrainElectric,
	movePsychicTerrain:  terrainPsychic,
}

var terrainName = map[int]string{
	terrainGrassy: "Травяная арена", terrainMisty: "Туманная арена",
	terrainElectric: "Электрическая арена", terrainPsychic: "Ментальная арена",
}

// setTerrain — поставить арену (взаимоисключающе, 5 ходов).
func setTerrain(bs *BattleState, terrain int, round int, log *[]BattleLogEntry) {
	bs.TerrainID = terrain
	bs.TerrainTurns = terrainDuration
	bs.TerrainSetTurn = round
	*log = append(*log, BattleLogEntry{Round: round, Message: terrainName[terrain] + " покрывает поле боя!"})
}

// terrainDamageMult — множитель урона от активной арены для данного хода.
func terrainDamageMult(bs *BattleState, move *BattleMove) float64 {
	if bs == nil || bs.TerrainTurns <= 0 {
		return 1.0
	}
	switch bs.TerrainID {
	case terrainGrassy:
		if move.TypeID == 5 { // Трава
			return 1.5
		}
		if move.ID == moveEarthquake { // Землетрясение ослаблено
			return 0.5
		}
	case terrainElectric:
		if move.TypeID == 4 { // Электро
			return 1.5
		}
	case terrainPsychic:
		if move.TypeID == 11 { // Психика
			return 1.5
		}
	case terrainMisty:
		if move.TypeID == 15 { // Дракон
			return 0.5
		}
	}
	return 1.0
}

// terrainBlocksPriority — Ментальная арена блокирует приоритетные урон-атаки (prio>0).
func terrainBlocksPriority(bs *BattleState, move *BattleMove) bool {
	return bs != nil && bs.TerrainTurns > 0 && bs.TerrainID == terrainPsychic && move.Priority > 0
}

// terrainBlocksStatus — можно ли наложить мажорный статус statusID при активной
// арене: Туманная блокирует любой (2-7), Электрическая — только сон (5).
func terrainBlocksStatus(bs *BattleState, statusID int) bool {
	if bs == nil || bs.TerrainTurns <= 0 {
		return false
	}
	if bs.TerrainID == terrainMisty && statusID >= 2 && statusID <= 7 {
		return true
	}
	if bs.TerrainID == terrainElectric && statusID == 5 {
		return true
	}
	return false
}

// tickTerrain — отсчёт арены в конце хода (в раунд установки не тикаем); Травяная
// в конце хода лечит обоих активных на 1/16 макс. HP.
func tickTerrain(bs *BattleState, log *[]BattleLogEntry) {
	if bs.TerrainTurns <= 0 {
		return
	}
	// Травяная арена лечит обоих активных.
	if bs.TerrainID == terrainGrassy {
		grassyHeal(&bs.PlayerTeam[bs.ActiveIdx], log)
		grassyHeal(bs.WildPokemon, log)
	}
	if bs.Turn == bs.TerrainSetTurn {
		return
	}
	bs.TerrainTurns--
	if bs.TerrainTurns == 0 {
		*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: terrainName[bs.TerrainID] + " рассеялась."})
		bs.TerrainID = terrainNone
	}
}

func grassyHeal(p *BattlePokemon, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted || p.CurrentHP >= p.MaxHP || p.HealBlockTurns > 0 {
		return
	}
	heal := p.MaxHP / 16
	if heal < 1 {
		heal = 1
	}
	p.CurrentHP += heal
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	*log = append(*log, BattleLogEntry{Round: 0, Message: p.Name + " лечится Травяной ареной! +" + strconv.Itoa(heal) + " HP"})
}
