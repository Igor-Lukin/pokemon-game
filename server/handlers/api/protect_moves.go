package api

import (
	mathrand "math/rand"
	"strconv"
)

// protect_moves.go — защитные ходы «на текущий ход» (Шаг A: щиты от урона).
//   71  Защита (Protect)         — блок любой атаки; счётчик повтора.
//   474 Засечка (Detect)         — то же, что Защита.
//   482 Каштановый щит (Spiky Shield) — блок + 1/8 макс.HP контактному атакующему.
//   404 Королевский щит (King's Shield) — блок + Атака-2 контактному атакующему.
//   537 Терпение (Endure)        — НЕ блок: переживает удар с минимум 1 HP; счётчик.
//   535 Татами Блок (Mat Block)  — блок ТОЛЬКО урон-атак, лишь в ПЕРВЫЙ ход после
//                                  выхода покемона (ActiveTurns==1); без счётчика.
//   361 Магический плащ (Magic Coat) — ОТРАЖАЕТ вредный статусный ход противника
//                                  обратно на него (статус/волатиль в applyEffectByID,
//                                  снижение статов в applyStatChanges). Priority 4.
// (Волшебный щит(359) блокирует статусные ходы; Магический плащ их отражает.)
//
// Механика: ход-защита имеет высокий приоритет (ходит первой) и ставит флаг на
// пользователя; удар противника в этот ход проверяет флаг и проваливается.
// Флаги снимаются в конце хода (endTurnProtect). Счётчик повтора (только Защита/
// Засечка/Терпение): 100%, а если В ПРОШЛОМ ходу защита удалась — 50%; после
// провала или хода без защиты снова 100%.

const (
	moveProtect     = 71
	moveDetect      = 474
	moveSpikyShield = 482
	moveKingShield  = 404
	moveCraftyShield = 359
	moveEndure      = 537
	moveMatBlock    = 535
	moveMagicCoat   = 361 // Магический плащ: отражает статусные ходы противника
	moveSnatch      = 466 // Грабёж: крадёт положительный статусный эффект противника
	spikyContactDenom = 8 // Каштановый: 1/8 макс. HP контактнику
	kingAttackDrop    = 2 // Королевский: Атака -2 контактнику
)

// handleProtectMove — постановка защитного хода (Шаг A). Возвращает true, если
// ход обработан. Учитывает счётчик повтора для Защиты/Засечки/Терпения.
func handleProtectMove(user *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	var flag *bool
	var msg string
	counted := false
	switch move.ID {
	case moveProtect, moveDetect:
		flag, msg, counted = &user.Protected, "приготовился защищаться!", true
	case moveSpikyShield:
		flag, msg = &user.SpikyShield, "выставил каштановый щит!"
	case moveKingShield:
		flag, msg = &user.KingShield, "поднял королевский щит!"
	case moveCraftyShield:
		flag, msg = &user.CraftyShield, "поднял волшебный щит — статусы не пройдут!"
	case moveMagicCoat:
		flag, msg = &user.MagicCoat, "накинул магический плащ — статусные ходы отразятся!"
	case moveSnatch:
		flag, msg = &user.Snatching, "притаился для грабежа — украдёт усиление/лечение противника!"
	case moveEndure:
		flag, msg, counted = &user.Enduring, "готов вытерпеть удар!", true
	case moveMatBlock:
		// Работает только в ПЕРВЫЙ ход покемона на поле (после выхода в бой).
		if user.ActiveTurns != 1 {
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: user.Name,
				MoveFailed: "Татами Блок работает только сразу после выхода — не сработало!",
			})
			return true
		}
		flag, msg = &user.MatBlocking, "расстелил татами — урон противника не пройдёт!"
	default:
		return false
	}
	chance := 100
	if counted && user.ProtectPrimed {
		chance = 50
	}
	if mathrand.Intn(100) < chance {
		*flag = true
		if counted {
			user.protectPrimeNext = true // удачная защита -> в следующий ход шанс 50%
		}
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: user.Name, PokemonImage: user.ImageURL,
			Message: user.Name + " " + msg,
		})
	} else {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: user.Name, PokemonImage: user.ImageURL,
			Message: "Но защита провалилась!",
		})
	}
	return true
}

// defenderBlocksDamage — стоит ли на защитнике щит, отражающий урон-атаку
// (Защита/Засечка/Каштановый/Королевский). Терпение НЕ блокирует (см. absorbWithEndure).
func defenderBlocksDamage(d *BattlePokemon) bool {
	return d.Protected || d.SpikyShield || d.KingShield || d.MatBlocking
}

// defenderBlocksStatus — стоит ли на защитнике щит, отражающий offensive
// СТАТУСНЫЙ ход противника (наложение статуса/снижение статов и т.п.). Блокируют
// все щиты, включая Волшебный (который блокирует ТОЛЬКО статусные).
func defenderBlocksStatus(d *BattlePokemon) bool {
	return d.Protected || d.SpikyShield || d.KingShield || d.CraftyShield
}

// applyContactPunish — наказание контактному атакующему за удар по щиту:
// Каштановый снимает 1/8 макс. HP, Королевский снижает Атаку на 2. Только для
// контактных атак (move.Contact).
func applyContactPunish(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if !move.Contact {
		return
	}
	if defender.SpikyShield {
		dmg := attacker.MaxHP / spikyContactDenom
		if dmg < 1 {
			dmg = 1
		}
		attacker.CurrentHP -= dmg
		if attacker.CurrentHP < 0 {
			attacker.CurrentHP = 0
		}
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " напоролся на шипы щита! -" + strconv.Itoa(dmg) + " HP",
		})
		if attacker.CurrentHP <= 0 {
			attacker.IsFainted = true
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, Fainted: true})
		}
	}
	if defender.KingShield {
		attacker.StatStages[1] -= kingAttackDrop
		if attacker.StatStages[1] < -6 {
			attacker.StatStages[1] = -6
		}
		*log = append(*log, BattleLogEntry{
			Round:      round,
			StatChange: "Атака " + attacker.Name + " резко снижена королевским щитом!",
		})
	}
}

// absorbWithEndure — если урон добил защитника, а у него активно Терпение,
// оставляем 1 HP. Вызывается сразу после вычитания HP (до определения обморока).
func absorbWithEndure(d *BattlePokemon, round int, log *[]BattleLogEntry) {
	if d.CurrentHP <= 0 && d.Enduring {
		d.CurrentHP = 1
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: d.Name, PokemonImage: d.ImageURL,
			Message: d.Name + " держится на ногах — остаётся 1 HP!",
		})
	}
}

// endTurnProtect — конец хода: перенос «удачной защиты» на приоритет следующего
// хода и снятие всех щитовых флагов.
func endTurnProtect(p *BattlePokemon) {
	p.ProtectPrimed = p.protectPrimeNext
	p.protectPrimeNext = false
	p.Protected = false
	p.SpikyShield = false
	p.KingShield = false
	p.CraftyShield = false
	p.Enduring = false
	p.MatBlocking = false
	p.MagicCoat = false
	p.Snatching = false
}

// isBounceableEffect — отражается ли эффект Магическим плащом: мажорные статусы
// (2-7), volatile (пиявки/ловушка/замешательство) и запрет смены (Пристальный взгляд).
func isBounceableEffect(id int) bool {
	if _, ok := statusRegistry[id]; ok {
		return true
	}
	if _, ok := inflictVolatile[id]; ok {
		return true
	}
	return id == effectMeanLook
}

// reflectEffectOnto — применить отражённый Плащом эффект к цели (каст-применившему).
func reflectEffectOnto(target *BattlePokemon, id, round int, log *[]BattleLogEntry) {
	if _, ok := statusRegistry[id]; ok {
		inflictMajorStatus(target, id, round, log)
		return
	}
	if fn, ok := inflictVolatile[id]; ok {
		fn(target, round, log)
		return
	}
	if id == effectMeanLook {
		inflictMeanLook(target, round, log)
	}
}
