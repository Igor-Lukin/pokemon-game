package api

import "strconv"

// counter_moves.go — контратака: вернуть противнику полученный за ход урон вдвое.
//   57 Контрудар (Counter) — возвращает ФИЗИЧЕСКИЙ урон ×2 (priority -5, ходит
//      последним, поэтому урон противника этого хода уже записан).
// Спец-версия (Mirror Coat) в БД отсутствует; чтобы её добавить — завести атаку и
// вписать сюда {её_id: 2} (возврат специального урона). Инфраструктура готова:
// executeDamagingMove пишет DamageTakenPhys/Spec цели, processTurn их обнуляет.

// counterMoves — id хода -> категория урона, который он возвращает (1 физ / 2 спец).
var counterMoves = map[int]int{
	57: 1, // Контрудар — физический
}

const moveBide = 461 // Выжидание

// recordDamageTaken — накопить полученный урон по категории атаки (Контрудар) и
// суммарно за раунды ожидания (Выжидание).
func recordDamageTaken(defender *BattlePokemon, move *BattleMove, dmg int) {
	if dmg <= 0 || defender == nil {
		return
	}
	switch move.CategoryID {
	case 1:
		defender.DamageTakenPhys += dmg
	case 2:
		defender.DamageTakenSpec += dmg
	}
	if defender.BideTurns > 0 { // Выжидание копит ВЕСЬ урон (физ+спец)
		defender.BideDamage += dmg
	}
}

// handleBideMove — Выжидание: первый вызов затаивается на 2 раунда (BideTurns=2),
// копит весь полученный урон (recordDamageTaken), на исходе ожидания возвращает
// его ×2 противнику. Форсится в processTurn как мультиход-лок.
func handleBideMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if move.ID != moveBide {
		return false
	}
	if attacker.BideTurns == 0 { // первый ход — затаиваемся
		attacker.BideTurns = 2
		attacker.BideDamage = 0
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " затаился и копит силы...",
		})
		return true
	}
	attacker.BideTurns--
	if attacker.BideTurns > 0 { // ещё ждём
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " продолжает копить силы...",
		})
		return true
	}
	// Ожидание вышло — высвобождаем накопленное ×2.
	dmg := attacker.BideDamage * 2
	attacker.BideDamage = 0
	if dmg <= 0 || defender == nil {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, MoveFailed: "Но копить было нечего — Выжидание провалилось!"})
		return true
	}
	defender.CurrentHP -= dmg
	if defender.CurrentHP < 0 {
		defender.CurrentHP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
		Message: attacker.Name + " высвобождает энергию! " + defender.Name + " получает " + strconv.Itoa(dmg) + " урона!",
	})
	if defender.CurrentHP <= 0 {
		defender.IsFainted = true
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: defender.Name, Fainted: true})
	}
	return true
}

// handleCounterMove — контратака: если по пользователю в этот ход прошёл урон
// нужной категории, возвращает его ×2 противнику (типонезависимо, как фикс. урон).
func handleCounterMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	cat, ok := counterMoves[move.ID]
	if !ok {
		return false
	}
	if defender == nil {
		return true
	}
	taken := attacker.DamageTakenPhys
	if cat == 2 {
		taken = attacker.DamageTakenSpec
	}
	if taken <= 0 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name,
			MoveFailed: "Контрудар не сработал — по вам не было подходящей атаки!",
		})
		return true
	}
	dmg := taken * 2
	defender.CurrentHP -= dmg
	if defender.CurrentHP < 0 {
		defender.CurrentHP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
		Message: attacker.Name + " возвращает удар вдвое! " + defender.Name + " получает " + strconv.Itoa(dmg) + " урона!",
	})
	if defender.CurrentHP <= 0 {
		defender.IsFainted = true
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: defender.Name, Fainted: true})
	}
	return true
}
