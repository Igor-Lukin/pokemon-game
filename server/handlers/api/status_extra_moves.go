package api

// status_extra_moves.go — точечные статус-манипуляции:
//   375 Зевок (Yawn)         — цель засыпает в КОНЦЕ следующего хода (отложенный сон).
//   506 Отместка (Psycho Shift) — передать СВОЙ мажорный статус противнику.

const (
	moveYawn        = 375
	movePsychoShift = 506
)

// handleStatusExtraMove — Зевок / Отместка. true, если ход обработан.
func handleStatusExtraMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case moveYawn:
		if defender == nil {
			return true
		}
		if defender.StatusID != 1 {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: defender.Name, MoveFailed: "У цели уже есть статус — Зевок не подействовал!"})
			return true
		}
		if defender.YawnTurns > 0 {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: defender.Name, MoveFailed: "Цель уже зевает!"})
			return true
		}
		defender.YawnTurns = 1
		defender.YawnSetTurn = round
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
			Message: defender.Name + " начинает дремать — заснёт в конце следующего хода!",
		})
	case movePsychoShift:
		if attacker.StatusID < 2 || attacker.StatusID > 7 {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, MoveFailed: "Нечего передавать — у пользователя нет недуга!"})
			return true
		}
		if defender == nil {
			return true
		}
		if inflictMajorStatus(defender, attacker.StatusID, round, log) {
			clearMajorStatus(attacker)
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
				Message: attacker.Name + " передаёт свой недуг противнику!",
			})
		} else {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: defender.Name, MoveFailed: "У противника уже есть статус — передать нельзя!"})
		}
	default:
		return false
	}
	return true
}

// clearMajorStatus — снять мажорный статус и связанные счётчики (для Отместки).
func clearMajorStatus(p *BattlePokemon) {
	p.StatusID = 1
	p.SleepTurns = 0
	p.PoisonTurns = 0
}

// tickYawn — Зевок: на исходе дремоты цель засыпает (если статус ещё свободен).
// В раунд наложения не тикаем (приём *SetTurn).
func tickYawn(p *BattlePokemon, curTurn int, log *[]BattleLogEntry) {
	if p == nil || p.YawnTurns <= 0 || curTurn == p.YawnSetTurn || p.IsFainted {
		return
	}
	p.YawnTurns--
	if p.YawnTurns > 0 {
		return
	}
	if p.StatusID == 1 {
		if inflictMajorStatus(p, 5, curTurn, log) {
			*log = append(*log, BattleLogEntry{
				Round: curTurn, PokemonName: p.Name, PokemonImage: p.ImageURL,
				Message: p.Name + " не может сдержать зевоту и засыпает!",
			})
		}
	}
}
