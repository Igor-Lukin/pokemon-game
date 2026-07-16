package api

import "strconv"

// sacrifice_moves.go — «жертва / взаимный обморок». Дебаффы статов у этих ходов
// сделаны ДАННЫМИ (attack_stat_changes); здесь — только то, что данными не выразить.
//   407 Обморок (Memento)       — Атака/СпАтака врага -2 (данные) + пользователь в обморок.
//   528 Связь судеб (Destiny Bond) — если носитель гибнет от удара врага, гибнет и враг.
//   365 Скорбная песнь (Perish Song) — оба падают в обморок через 3 хода.
//   531 Слёзный взгляд (Tearful Look) — Атака/СпАтака врага -1: ПОЛНОСТЬЮ данными.

const (
	moveMemento     = 407
	moveDestinyBond = 528
	moveGrudge      = 469
	movePerishSong  = 365
	perishTurns     = 3
)

// handleSacrificeMove — жертвенные статусные ходы. true, если обработан.
func handleSacrificeMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case moveMemento:
		// Дебафф врага уже применён applyStatChanges; пользователь падает в обморок.
		attacker.CurrentHP = 0
		attacker.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " приносит себя в жертву!",
		})
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, Fainted: true})
	case moveDestinyBond:
		attacker.DestinyBond = true
		attacker.DestinyBondTurn = round
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " готов забрать врага с собой!",
		})
	case moveGrudge:
		attacker.Grudge = true
		attacker.GrudgeTurn = round
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " затаил обиду — павший заберёт PP убийцы!",
		})
	case movePerishSong:
		if defender == nil {
			return true
		}
		setPerish(attacker, round)
		setPerish(defender, round)
		*log = append(*log, BattleLogEntry{
			Round: round, Message: "Звучит скорбная песнь — оба монстра падут через " + strconv.Itoa(perishTurns) + " хода!",
		})
	default:
		return false
	}
	return true
}

func setPerish(p *BattlePokemon, round int) {
	if p.PerishCount > 0 {
		return // уже под действием
	}
	p.PerishCount = perishTurns
	p.PerishSetTurn = round
}

// applyDestinyBond — вызывается после того, как удар отправил defender в обморок:
// если у него была Связь судеб, атакующий гибнет тоже.
func applyDestinyBond(attacker, defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	if !defender.DestinyBond || attacker.IsFainted {
		return
	}
	attacker.CurrentHP = 0
	attacker.IsFainted = true
	*log = append(*log, BattleLogEntry{
		Round: round, Message: defender.Name + " забирает " + attacker.Name + " с собой Связью судеб!",
	})
	*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, Fainted: true})
}

// tickPerish — обратный отсчёт Скорбной песни в конце хода (не в раунд установки);
// на нуле носитель падает в обморок.
func tickPerish(p *BattlePokemon, curTurn int, log *[]BattleLogEntry) {
	if p == nil || p.PerishCount <= 0 || curTurn == p.PerishSetTurn || p.IsFainted {
		return
	}
	p.PerishCount--
	if p.PerishCount > 0 {
		*log = append(*log, BattleLogEntry{
			Round: curTurn, Message: p.Name + ": скорбная песнь — осталось " + strconv.Itoa(p.PerishCount) + " хода.",
		})
		return
	}
	p.CurrentHP = 0
	p.IsFainted = true
	*log = append(*log, BattleLogEntry{Round: curTurn, Message: p.Name + " падает от скорбной песни!"})
	*log = append(*log, BattleLogEntry{Round: curTurn, PokemonName: p.Name, Fainted: true})
}

// tickDestinyBond — Связь судеб держится до конца СЛЕДУЮЩЕГО хода после установки.
func tickDestinyBond(p *BattlePokemon, curTurn int, log *[]BattleLogEntry) {
	if p == nil || !p.DestinyBond || curTurn == p.DestinyBondTurn {
		return
	}
	p.DestinyBond = false
}

// applyGrudge — после того как удар отправил defender в обморок: если у него был
// Жалостливый взор, у хода, которым его добили, обнуляется PP.
func applyGrudge(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if !defender.Grudge || move == nil {
		return
	}
	if move.PP > 0 {
		move.PP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round: round, Message: defender.Name + " напоследок стирает все PP хода «" + move.Name + "»!",
	})
}

// tickGrudge — Жалостливый взор держится до конца СЛЕДУЮЩЕГО хода после установки.
func tickGrudge(p *BattlePokemon, curTurn int, log *[]BattleLogEntry) {
	if p == nil || !p.Grudge || curTurn == p.GrudgeTurn {
		return
	}
	p.Grudge = false
}
