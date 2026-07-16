package api

// block_moves.go — заставить/блокировать ходы противника (группа 2 кластера).
//   383 Овации (Encore)          — цель форсированно повторяет последний ход 3 хода.
//   386 Подстрекательство (Torment) — цель не может повторять ход два раза подряд.
//   471 Запрет (Disable)         — последний ход цели запрещён 4 хода (нужно ≥2 ходов).
//   515 Приказ (Instruct)        — цель повторяет последний ход в следующем раунде.
//   501 Обезоруживание (Spite)   — PP последнего хода цели -4.
//   381 Насмешка (Taunt)         — цель не может использовать статусные ходы 3 хода.
//   518 Потеря памяти (Imprison) — цель не может использовать ходы, которые знает
//                                  пользователь (снимок id набора заклинателя).
// Опора: BattlePokemon.LastUsedMoveID (ставится в markMoveUsed). *MoveID/Tormented
// уходят на фронт для грейаута/подсветки. Форсинг Овациями — в processTurn/
// chooseWildMoveOrCharge; блок выбора — moveBlockedFor (серверная страховка в
// MoveHandler + исключение из выбора дикого; фронт-грейаут Насмешки/Потери памяти
// пока не подключён — недоступный ход просто отклоняется сервером).

const (
	moveEncore   = 383
	moveTorment  = 386
	moveDisable  = 471
	moveInstruct = 515
	moveSpite    = 501
	moveTaunt    = 381 // Насмешка: цель не может использовать статусные ходы 3 хода
	moveImprison = 518 // Потеря памяти: цель не может использовать ходы, что знает юзер
	tauntDuration = 3
)

// handleBlockMove — статусные ходы блока/форса. Возвращает true, если обработан.
func handleBlockMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil {
		return false
	}
	switch move.ID {
	case moveEncore:
		if defender.LastUsedMoveID == 0 {
			blockFailLog(defender, "Нечего повторять — Овации не сработали!", round, log)
			return true
		}
		defender.EncoreMoveID = defender.LastUsedMoveID
		defender.EncoreTurns = 3
		blockLog(defender, defender.Name+" вынужден повторять свою атаку!", round, log)
	case moveInstruct:
		if defender.LastUsedMoveID == 0 {
			blockFailLog(defender, "Нечего приказать повторить!", round, log)
			return true
		}
		defender.EncoreMoveID = defender.LastUsedMoveID
		defender.EncoreTurns = 1
		blockLog(defender, defender.Name+" повторит свою атаку в следующем ходу!", round, log)
	case moveTorment:
		defender.Tormented = true
		blockLog(defender, defender.Name+" раздражён — не сможет повторять атаку подряд!", round, log)
	case moveDisable:
		if defender.DisableTurns > 0 {
			blockFailLog(defender, "Запрет уже действует!", round, log)
			return true
		}
		if defender.LastUsedMoveID == 0 || countMoves(defender) < 2 {
			blockFailLog(defender, "Запрет не сработал!", round, log)
			return true
		}
		defender.DisabledMoveID = defender.LastUsedMoveID
		defender.DisableTurns = 4
		blockLog(defender, "Последняя атака "+defender.Name+" запрещена!", round, log)
	case moveSpite:
		if m := findMoveByID(defender, defender.LastUsedMoveID); m != nil && m.PP >= 0 {
			m.PP -= 4
			if m.PP < 0 {
				m.PP = 0
			}
			blockLog(defender, "PP последней атаки "+defender.Name+" снижены!", round, log)
		} else {
			blockFailLog(defender, "Обезоруживание не сработало!", round, log)
		}
	case moveTaunt:
		if defender.TauntTurns > 0 {
			blockFailLog(defender, "Насмешка уже действует!", round, log)
			return true
		}
		defender.TauntTurns = tauntDuration
		blockLog(defender, defender.Name+" разозлён — не сможет использовать статусные ходы!", round, log)
	case moveImprison:
		// Запретить цели все ходы, которые знает пользователь (снимок id).
		ids := make([]int, 0, len(attacker.Moves))
		for i := range attacker.Moves {
			ids = append(ids, attacker.Moves[i].ID)
		}
		defender.ImprisonedMoveIDs = ids
		blockLog(defender, defender.Name+" не может использовать атаки, знакомые противнику!", round, log)
	default:
		return false
	}
	return true
}

func blockLog(p *BattlePokemon, msg string, round int, log *[]BattleLogEntry) {
	*log = append(*log, BattleLogEntry{Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL, Message: msg})
}
func blockFailLog(p *BattlePokemon, msg string, round int, log *[]BattleLogEntry) {
	*log = append(*log, BattleLogEntry{Round: round, PokemonName: p.Name, MoveFailed: msg})
}

func countMoves(p *BattlePokemon) int { return len(p.Moves) }

// resetBlockState — снять блок/форс-состояние (при смене покемона).
func resetBlockState(p *BattlePokemon) {
	p.EncoreMoveID = 0
	p.EncoreTurns = 0
	p.DisabledMoveID = 0
	p.DisableTurns = 0
	p.Tormented = false
	p.LastUsedMoveID = 0
	p.TauntTurns = 0
	p.ImprisonedMoveIDs = nil
}

// moveBlockedFor — нельзя ли покемону выбрать ход moveID сейчас: Запрет на этот
// ход; Подстрекательство (повтор последнего); Овации (любой, кроме форс-хода).
func moveBlockedFor(p *BattlePokemon, moveID int) bool {
	if p.DisableTurns > 0 && moveID == p.DisabledMoveID {
		return true
	}
	if p.Tormented && moveID == p.LastUsedMoveID {
		return true
	}
	if p.EncoreTurns > 0 && p.EncoreMoveID != 0 && moveID != p.EncoreMoveID {
		return true
	}
	// Насмешка — нельзя статусные ходы (кат.3). Категорию берём из набора покемона.
	if p.TauntTurns > 0 {
		if m := findMoveByID(p, moveID); m != nil && m.CategoryID == 3 {
			return true
		}
	}
	// Потеря памяти — запрещены конкретные ходы (что знает пользователь-заклинатель).
	for _, id := range p.ImprisonedMoveIDs {
		if id == moveID {
			return true
		}
	}
	return false
}

// tickBlockTimers — конец хода: убавляем таймеры Овации/Запрета, снимаем при 0.
func tickBlockTimers(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p.EncoreTurns > 0 {
		p.EncoreTurns--
		if p.EncoreTurns == 0 {
			p.EncoreMoveID = 0
		}
	}
	if p.DisableTurns > 0 {
		p.DisableTurns--
		if p.DisableTurns == 0 {
			p.DisabledMoveID = 0
			*log = append(*log, BattleLogEntry{Round: round, Message: "Запрет с " + p.Name + " спал."})
		}
	}
	if p.TauntTurns > 0 {
		p.TauntTurns--
		if p.TauntTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: round, Message: "Насмешка с " + p.Name + " спала."})
		}
	}
}
