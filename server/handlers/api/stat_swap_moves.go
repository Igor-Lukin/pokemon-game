package api

// stat_swap_moves.go — статусные ходы, работающие с БАЗОВЫМИ статами (не со
// ступенями StatStages, теми занимается copy_moves.go). Меняют/усредняют поля
// Attack/Defense/Speed/SpecialAttack/SpecialDefense напрямую — эти поля читает
// calculateDamage (через effectiveStat) и порядок хода (Speed), поэтому эффект
// сразу виден в бою.
//
// Так как это базовые статы (а не волатили), при уходе покемона их надо вернуть
// в исходное — иначе обмен «прилипнет» к покемону на скамейке. Для этого при
// загрузке снимается снимок Base* (snapshotBaseStats), а смена восстанавливает
// его (restoreBaseStats, зовётся из обоих мест сброса при смене).
//   492 Ловкая подмена (Power Trick) — обмен СВОИХ Атаки<->Защиты.
//   514 Подмена скорости (Speed Swap) — обмен Скорости с противником.
//   522 Разделение атаки (Power Split) — усреднить Атаку и Спец.Атаку с противником.
//   523 Разделение защиты (Guard Split) — усреднить Защиту и Спец.Защиту с противником.

const (
	movePowerTrick = 492
	moveSpeedSwap  = 514
	movePowerSplit = 522
	moveGuardSplit = 523
)

// snapshotBaseStats — запомнить исходные базовые статы (при загрузке покемона).
func snapshotBaseStats(p *BattlePokemon) {
	p.BaseAttack = p.Attack
	p.BaseDefense = p.Defense
	p.BaseSpeed = p.Speed
	p.BaseSpecialAttack = p.SpecialAttack
	p.BaseSpecialDefense = p.SpecialDefense
}

// restoreBaseStats — вернуть базовые статы к снимку (при уходе покемона), чтобы
// обмен/усреднение не переносились на будущее.
func restoreBaseStats(p *BattlePokemon) {
	p.Attack = p.BaseAttack
	p.Defense = p.BaseDefense
	p.Speed = p.BaseSpeed
	p.SpecialAttack = p.BaseSpecialAttack
	p.SpecialDefense = p.BaseSpecialDefense
}

// avgStat — уравнять два стата по среднему арифметическому (Power/Guard Split).
func avgStat(a, b *int) {
	m := (*a + *b) / 2
	*a, *b = m, m
}

// handleStatSwapMove — обмен/усреднение базовых статов. true, если ход обработан.
func handleStatSwapMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case movePowerTrick:
		attacker.Attack, attacker.Defense = attacker.Defense, attacker.Attack
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " поменял местами свои Атаку и Защиту!",
		})
	case moveSpeedSwap:
		if defender == nil {
			return true
		}
		attacker.Speed, defender.Speed = defender.Speed, attacker.Speed
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " обменялся Скоростью с противником!",
		})
	case movePowerSplit:
		if defender == nil {
			return true
		}
		avgStat(&attacker.Attack, &defender.Attack)
		avgStat(&attacker.SpecialAttack, &defender.SpecialAttack)
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " уравнял Атаку и Спец.Атаку с противником!",
		})
	case moveGuardSplit:
		if defender == nil {
			return true
		}
		avgStat(&attacker.Defense, &defender.Defense)
		avgStat(&attacker.SpecialDefense, &defender.SpecialDefense)
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " уравнял Защиту и Спец.Защиту с противником!",
		})
	default:
		return false
	}
	return true
}
