package api

import "strconv"

// future_moves.go — отложенные атаки (Future Sight): урон наносится через 2
// раунда после применения, независимо от того, что делает пользователь.
//   316 Светопреставление (Steel, 140)
//   303 Предсказание       (Psychic, 120)
//
// При запуске урон СРАЗУ рассчитывается (снимок сил на момент запуска) и
// сохраняется на ЦЕЛИ вместе со счётчиком. Так атака сработает даже если
// пользователь сменится/падёт. Урон применяется тому, кто стоит в слоте цели
// на момент срабатывания (упрощение: величина не пересчитывается).

var futureMoves = map[int]bool{
	316: true, // Светопреставление
	303: true, // Предсказание
}

// scheduleFutureAttack — запланировать отложенную атаку на defender. round —
// текущий ход (bs.Turn). Возвращает управление без немедленного урона.
func scheduleFutureAttack(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if defender.FutureMoveID != 0 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name,
			Message: "Но отложенная атака уже нацелена на цель!",
		})
		return
	}
	dmg, _, _ := calculateDamage(nil, attacker, defender, move, nil, round, log) // снимок урона; поле не учитываем
	if dmg < 1 {
		dmg = 1
	}
	defender.FutureMoveID = move.ID
	defender.FutureTurns = 2
	defender.FutureSetTurn = round
	defender.FutureDamage = dmg
	defender.FutureName = move.Name
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
		Message: attacker.Name + " готовит отложенную атаку «" + move.Name + "»!",
	})
}

// applyFutureAttacks — конец хода: тикнуть и применить созревшие отложенные
// атаки на обоих покемонах.
func applyFutureAttacks(bs *BattleState, a, b *BattlePokemon, log *[]BattleLogEntry) {
	resolveFutureAttack(bs, a, log)
	resolveFutureAttack(bs, b, log)
}

func resolveFutureAttack(bs *BattleState, p *BattlePokemon, log *[]BattleLogEntry) {
	if p.FutureMoveID == 0 {
		return
	}
	// В ход запуска счётчик не уменьшаем.
	if bs.Turn == p.FutureSetTurn {
		return
	}
	p.FutureTurns--
	if p.FutureTurns > 0 {
		return
	}
	dmg := p.FutureDamage
	name := p.FutureName
	p.FutureMoveID = 0
	p.FutureTurns = 0
	p.FutureDamage = 0
	p.FutureName = ""
	if p.IsFainted {
		return
	}
	p.CurrentHP -= dmg
	if p.CurrentHP < 0 {
		p.CurrentHP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round: bs.Turn, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " поражён отложенной атакой «" + name + "»! -" + strconv.Itoa(dmg) + " HP",
	})
	if p.CurrentHP <= 0 {
		p.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round: bs.Turn, PokemonName: p.Name, Fainted: true,
		})
	}
}
