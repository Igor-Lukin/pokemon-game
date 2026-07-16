package api

// room_moves.go — «арены-довесок»: комнаты/поле и связанное.
//   476 Ионное поле (Ion Deluge)  — 5р: Нормальные атаки становятся Электрическими.
//   486 Комната смеха (Trick Room) — 5р: медленные ходят первыми (инверсия порядка).
//   487 Комната чудес (Wonder Room) — 5р: Защита<->СпЗащита у всех (в расчёте урона).
//   509 Плантаж (Rototiller)      — МГНОВЕННО: Травяным активным +1 Атака/+1 СпАтака.
//   529 Сила природы (Nature Power) — применяет атаку по типу активной арены.
// Таймеры — на BattleState (независимые); постановка — applySideMove; отсчёт —
// tickSideTimers. Сила природы/Плантаж — не таймеры (выполняются сразу).

const (
	moveIonDeluge   = 476
	moveTrickRoom   = 486
	moveWonderRoom  = 487
	moveRototiller  = 509
	moveNaturePower = 529
	roomDuration    = 5
	typeGrass       = 5
)

// handleRoomMove — комнаты/поле/Плантаж (кроме Силы природы — она в handleCallMove).
// true, если ход обработан.
func handleRoomMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, log *[]BattleLogEntry) bool {
	switch move.ID {
	case moveIonDeluge:
		bs.IonDelugeTurns = roomDuration
		bs.IonDelugeSetTurn = bs.Turn
		*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Поле наэлектризовано ионами — Нормальные атаки станут Электрическими!"})
	case moveTrickRoom:
		bs.TrickRoomTurns = roomDuration
		bs.TrickRoomSetTurn = bs.Turn
		*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Искажается пространство — медленные монстры ходят первыми!"})
	case moveWonderRoom:
		bs.WonderRoomTurns = roomDuration
		bs.WonderRoomSetTurn = bs.Turn
		*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Комната чудес меняет местами Защиту и Спец.Защиту!"})
	case moveRototiller:
		rototillerBoost(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
		rototillerBoost(bs.WildPokemon, bs.Turn, log)
	default:
		return false
	}
	return true
}

// rototillerBoost — Плантаж: Травяному активному +1 Атака/+1 СпАтака.
func rototillerBoost(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted || !pokemonHasType(p, typeGrass) {
		return
	}
	curseBump(p, 1, 1, round, log) // Атака +1
	curseBump(p, 4, 1, round, log) // СпАтака +1
}

// ionDelugeAdjust — Ионное поле: копия хода с типом Электро для Нормальных атак.
func ionDelugeAdjust(bs *BattleState, move *BattleMove) *BattleMove {
	if bs == nil || bs.IonDelugeTurns <= 0 || move.TypeID != 1 {
		return move
	}
	m := *move
	m.TypeID = typeElectric
	return &m
}

// tickRooms — отсчёт комнат/поля в конце хода (в раунд установки не тикаем).
func tickRooms(bs *BattleState, log *[]BattleLogEntry) {
	if bs.IonDelugeTurns > 0 && bs.Turn != bs.IonDelugeSetTurn {
		bs.IonDelugeTurns--
		if bs.IonDelugeTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Ионное поле рассеялось."})
		}
	}
	if bs.TrickRoomTurns > 0 && bs.Turn != bs.TrickRoomSetTurn {
		bs.TrickRoomTurns--
		if bs.TrickRoomTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Комната смеха исчезла — порядок хода восстановлен."})
		}
	}
	if bs.WonderRoomTurns > 0 && bs.Turn != bs.WonderRoomSetTurn {
		bs.WonderRoomTurns--
		if bs.WonderRoomTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Комната чудес исчезла."})
		}
	}
}

// naturePowerMoveID — Сила природы: id атаки по активной арене (нет арены -> Свист/
// Норм). Возвращает id ходовой атаки для executeCalledMove.
func naturePowerMoveID(bs *BattleState) int {
	if bs != nil && bs.TerrainTurns > 0 {
		switch bs.TerrainID {
		case terrainGrassy:
			return 28 // Вихрь лепестков (Трава)
		case terrainElectric:
			return 351 // Электрический шар (Электро)
		case terrainPsychic:
			return 305 // Психическая волна (Психо)
		case terrainMisty:
			return 234 // Волшебный ветер (Фея)
		}
	}
	return 250 // Звуковая волна (Норм, фикс. урон) — фолбэк без арены
}
