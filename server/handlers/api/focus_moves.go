package api

import mathrand "math/rand"

// focus_moves.go — «подготовительные» статусные ходы: самобусты статов, гарантия
// попадания/крита следующей атаки и «наводка» (сброс уклонения цели + снятие
// иммунитета Призраков/Тёмных). Фиксированные прибавки статов сделаны ДАННЫМИ
// (attack_stat_changes); здесь — только то, что данными не выразить.
//   58  Набат (Belly Drum)          — Атака сразу до +6 ценой половины макс. HP.
//   435 Массаж                      — случайная характеристика +2.
//   473 Заряд (Charge)              — флаг: следующая электрическая атака ×2 (СпЗащ+1 — данные).
//   444 Сброс балласта              — вес пользователя вдвое (Скорость+2 — данные).
//   521 Прицел / 534 Сосредоточение — следующая атака гарантированно попадает.
//   543 Ультрафокусировка           — следующая атака гарантированно критическая.
//   502 Обнаружение / 545 Чутьё     — сброс уклонения цели + Норм/Боевой бьют Призраков.
//   538 Третий глаз                 — сброс уклонения цели + Психика бьёт Тёмных.

const (
	moveBellyDrum  = 58
	moveMassage    = 435
	moveCharge     = 473
	moveDitchBallast = 444
	moveLockOn     = 521
	moveMindReader = 534
	moveLaserFocus = 543
	moveForesight  = 502
	moveOdorSleuth = 545
	moveMiracleEye = 538
)

// handleFocusMove — подготовительные статусные ходы. true, если обработан.
func handleFocusMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case moveBellyDrum:
		bellyDrumMove(attacker, round, log)
	case moveMassage:
		curseBump(attacker, mathrand.Intn(5)+1, 2, round, log) // случайная хар-ка (1..5) +2
	case moveCharge:
		attacker.ChargedElectric = true
		focusLog(attacker, attacker.Name+" заряжается — следующая электрическая атака усилена!", round, log)
	case moveDitchBallast:
		if attacker.Weight > 0 {
			attacker.Weight /= 2
		}
		focusLog(attacker, attacker.Name+" сбрасывает балласт и становится легче!", round, log)
	case moveLockOn, moveMindReader:
		attacker.SureHit = true
		focusLog(attacker, attacker.Name+" прицеливается — следующая атака не промахнётся!", round, log)
	case moveLaserFocus:
		attacker.SureCrit = true
		focusLog(attacker, attacker.Name+" предельно сосредоточен — следующая атака будет критической!", round, log)
	case moveForesight, moveOdorSleuth:
		if defender == nil {
			return true
		}
		defender.Foresighted = true
		defender.StatStages[7] = 0 // сброс уклонения
		focusLog(defender, defender.Name+" обнаружен — уклонение сброшено, теперь уязвим для Нормальных и Боевых атак!", round, log)
	case moveMiracleEye:
		if defender == nil {
			return true
		}
		defender.MiracleEyed = true
		defender.StatStages[7] = 0
		focusLog(defender, defender.Name+" под наблюдением — уклонение сброшено, теперь уязвим для Психических атак!", round, log)
	default:
		return false
	}
	return true
}

func focusLog(p *BattlePokemon, msg string, round int, log *[]BattleLogEntry) {
	*log = append(*log, BattleLogEntry{Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL, Message: msg})
}

// bellyDrumMove — Набат: доводит Атаку до максимума (+6) ценой половины макс. HP.
// Провал, если Атака уже на максимуме или HP не хватает на плату.
func bellyDrumMove(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	cost := p.MaxHP / 2
	if p.StatStages[1] >= 6 {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: p.Name, MoveFailed: "Атака уже на максимуме — не сработало!"})
		return
	}
	if p.CurrentHP <= cost {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: p.Name, MoveFailed: "Недостаточно HP для Набата!"})
		return
	}
	p.CurrentHP -= cost
	p.StatStages[1] = 6
	focusLog(p, p.Name+" жертвует половиной HP и доводит Атаку до предела!", round, log)
}

// consumeFocusOnAttack — израсходовать одноразовые подготовительные флаги после
// нанесённого удара (вызывается в конце executeDamagingMove).
func consumeFocusOnAttack(p *BattlePokemon) {
	p.SureHit = false
	p.SureCrit = false
	p.ChargedElectric = false
}

// effectivenessIgnoringImmunity — эффективность по типу с игнорированием ОДНОГО
// типа-защитника (снятие иммунитета Обнаружением/Третьим глазом): игнорируемый
// тип не вносит вклад, прочие считаются штатно.
func effectivenessIgnoringImmunity(attackType int, defTypes []int, ignoreType int) float64 {
	mult := 1.0
	for _, dt := range defTypes {
		if dt == ignoreType {
			continue
		}
		mult *= getTypeEffectiveness(attackType, []int{dt})
	}
	return mult
}
