package api

import (
	mathrand "math/rand"
	"pokemon-game/server/db"
)

// call_moves.go — ходы, которые ВЫПОЛНЯЮТ другой ход (группа 1 кластера).
//   497 Метроном (Metronome)    — случайная любая атака.
//   485 Карикатура (Copycat)    — последний ход противника.
//   511 Поддержка (Assist)      — случайный ход из другого покемона команды.
//   532 Синхронный полёт (Mirror Move) — повторяет последний ход противника.
//   516 Подражание (Mimic)      — заменяет свой ход Подражание на последний ход противника.
//   550 Эскиз (Sketch)          — то же (в бою «выучивает» последний ход противника).
//   457 Бормотание (Sleep Talk) — во сне применяет случайный свой ход (sleep_moves.go
//       разрешает действие во сне; здесь — выбор и запуск хода).
// НЕ здесь: Me First(504) — требует пред-эмпции по порядку хода. Отложен.
//
// Опора: defender.LastUsedMoveID (block_moves.go/markMoveUsed) + loadBattleMoveByID.

// callerMoveIDs — ходы-«вызыватели»: исключаются из случайного пула (Метроном) и
// не рекурсируют (executeCalledMove их не диспетчеризует повторно).
var callerMoveIDs = map[int]bool{
	497: true, 485: true, 511: true, 532: true, 516: true, 550: true, 457: true, 504: true,
	209: true, 311: true, // Подставной ход/Ток — смена, не годятся для вызова
	529: true,            // Сила природы — вызыватель (room_moves.go)
}

// loadBattleMoveByID — собрать BattleMove из таблицы attacks по id (для вызванных
// ходов и Подражания/Эскиза). PP берётся из БД (для Подражания/Эскиза — конечный).
func loadBattleMoveByID(id int) *BattleMove {
	var m BattleMove
	var neverMisses, contact int
	err := db.DB.QueryRow(`
		SELECT id, name, COALESCE(eng_name,''), type_id, category_id, power, accuracy,
		       COALESCE(pp,0), priority, COALESCE(target_id,1), COALESCE(description,''),
		       COALESCE(effect_status_id,0), COALESCE(effect_chance,0), COALESCE(never_misses,0),
		       COALESCE(drain,0), COALESCE(crit_rate,0), COALESCE(hits_min,1), COALESCE(hits_max,1),
		       COALESCE(heal_percent,0), COALESCE(contact,0)
		FROM attacks WHERE id=?`, id).Scan(
		&m.ID, &m.Name, &m.EngName, &m.TypeID, &m.CategoryID, &m.Power, &m.Accuracy,
		&m.PP, &m.Priority, &m.TargetID, &m.Description, &m.EffectID, &m.EffectChance,
		&neverMisses, &m.Drain, &m.CritRate, &m.HitsMin, &m.HitsMax, &m.HealPercent, &contact)
	if err != nil {
		return nil
	}
	m.MaxPP = m.PP
	m.NeverMisses = neverMisses != 0
	m.Contact = contact != 0
	m.StatChanges = loadStatChanges(m.ID)
	return &m
}

// executeCalledMove — выполнить ход m от лица attacker по defender (урон или
// статус), как обычный ход. НЕ вызывает handleCallMove повторно (без рекурсии).
func executeCalledMove(bs *BattleState, attacker, defender *BattlePokemon, m *BattleMove, round int, log *[]BattleLogEntry) {
	if m == nil {
		return
	}
	if m.CategoryID != 3 && m.Power > 0 {
		fainted, wc := executeDamagingMove(bs, attacker, defender, m, bs.BattleWeatherID, round, log, sideFor(bs, defender), sideFor(bs, attacker), bs.WaterSportTurns > 0)
		if wc > 0 {
			setBattleWeather(bs, wc, log)
		}
		if fainted {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: defender.Name, Fainted: true})
		}
		return
	}
	*log = append(*log, makeEntry(round, attacker, m, 0, false, 1, false))
	if m.CategoryID == 3 {
		if wc := applyEffectByID(bs, attacker, defender, m, round, log, sideFor(bs, defender)); wc > 0 {
			setBattleWeather(bs, wc, log)
		}
		applyStatChanges(attacker, defender, m, bs.BattleWeatherID, round, log, sideFor(bs, defender))
		applyHeal(attacker, defender, m, bs.BattleWeatherID, round, log)
		handleCustomStatusMove(bs, attacker, defender, m, round, log)
		applySideMove(bs, attacker, defender, m, log)
	}
}

// handleCallMove — статусные ходы-вызыватели. Возвращает true, если обработан.
func handleCallMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case 497: // Метроном — случайный ход
		if m := loadBattleMoveByID(randomMetronomeMoveID()); m != nil {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL, Message: attacker.Name + " использует " + m.Name + " (Метроном)!"})
			executeCalledMove(bs, attacker, defender, m, round, log)
		}
	case 485, 532: // Copycat / Mirror Move — последний ход противника
		if defender == nil || defender.LastUsedMoveID == 0 {
			callFailLog(attacker, "Нечего копировать!", round, log)
			return true
		}
		if m := loadBattleMoveByID(defender.LastUsedMoveID); m != nil {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL, Message: attacker.Name + " копирует " + m.Name + "!"})
			executeCalledMove(bs, attacker, defender, m, round, log)
		}
	case 511: // Assist — случайный ход товарища по команде
		m := assistMove(bs, attacker)
		if m == nil {
			callFailLog(attacker, "Помощь не пришла — некому!", round, log)
			return true
		}
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL, Message: attacker.Name + " зовёт на помощь: " + m.Name + "!"})
		executeCalledMove(bs, attacker, defender, m, round, log)
	case 516, 550: // Mimic / Sketch — заменить свой ход-вызыватель на ход противника
		mimicSketch(attacker, defender, move.ID, round, log)
	case moveSleepTalk: // Бормотание — во сне применяет случайный свой ход (sleep_moves.go)
		if attacker.StatusID != 5 {
			callFailLog(attacker, "Бормотание работает только во сне!", round, log)
			return true
		}
		m := sleepTalkMove(attacker)
		if m == nil {
			callFailLog(attacker, "Бормотание не нашло подходящего хода!", round, log)
			return true
		}
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL, Message: attacker.Name + " бормочет во сне и применяет " + m.Name + "!"})
		executeCalledMove(bs, attacker, defender, m, round, log)
	case moveNaturePower: // Сила природы — атака по типу активной арены (room_moves.go)
		if m := loadBattleMoveByID(naturePowerMoveID(bs)); m != nil {
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL, Message: attacker.Name + " взывает к силе природы: " + m.Name + "!"})
			executeCalledMove(bs, attacker, defender, m, round, log)
		}
	default:
		return false
	}
	return true
}

func callFailLog(p *BattlePokemon, msg string, round int, log *[]BattleLogEntry) {
	*log = append(*log, BattleLogEntry{Round: round, PokemonName: p.Name, MoveFailed: msg})
}

const moveMeFirst = 504 // Опережение

// meFirstOK — можно ли Опережением скопировать ход foeMove: пользователь СТРОГО
// быстрее, противник может действовать, его ход — урон-атака (не статус) и не
// вызыватель/смена. Само Опережение (504) в callerMoveIDs, поэтому не копируется.
func meFirstOK(bs *BattleState, user, foe *BattlePokemon, foeCanAct bool, foeMove *BattleMove) bool {
	if !foeCanAct || foeMove == nil {
		return false
	}
	if foeMove.CategoryID == 3 || foeMove.Power <= 0 {
		return false // не урон-атака
	}
	if callerMoveIDs[foeMove.ID] {
		return false
	}
	return tailwindSpeed(bs, user) > tailwindSpeed(bs, foe) // пользователь быстрее
}

// randomMetronomeMoveID — случайный id включённой атаки, кроме вызывателей/смен.
func randomMetronomeMoveID() int {
	rows, err := db.DB.Query("SELECT id FROM attacks WHERE disabled=0")
	if err != nil {
		return 33 // фолбэк — какая-нибудь простая атака
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil && !callerMoveIDs[id] {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 33
	}
	return ids[mathrand.Intn(len(ids))]
}

// assistMove — случайный (не-вызыватель) ход из ДРУГОГО покемона команды игрока.
// Работает только для активного покемона игрока (у дикого команды нет).
func assistMove(bs *BattleState, attacker *BattlePokemon) *BattleMove {
	if attacker != &bs.PlayerTeam[bs.ActiveIdx] {
		return nil
	}
	var pool []BattleMove
	for i := range bs.PlayerTeam {
		if i == bs.ActiveIdx || bs.PlayerTeam[i].IsFainted {
			continue
		}
		for _, m := range bs.PlayerTeam[i].Moves {
			if !callerMoveIDs[m.ID] {
				pool = append(pool, m)
			}
		}
	}
	if len(pool) == 0 {
		return nil
	}
	m := pool[mathrand.Intn(len(pool))]
	return &m
}

// mimicSketch — Подражание(516)/Эскиз(550): заменить в наборе атакующего сам ход-
// вызыватель на последний ход противника. В бою эффект одинаков (Эскиз «навсегда»
// в рамках БД не сохраняется — только на текущий бой).
func mimicSketch(attacker, defender *BattlePokemon, selfID, round int, log *[]BattleLogEntry) {
	if defender == nil || defender.LastUsedMoveID == 0 {
		callFailLog(attacker, "Нечего запоминать!", round, log)
		return
	}
	learned := loadBattleMoveByID(defender.LastUsedMoveID)
	if learned == nil {
		return
	}
	for i := range attacker.Moves {
		if attacker.Moves[i].ID == selfID {
			attacker.Moves[i] = *learned
			*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL, Message: attacker.Name + " запомнил ход " + learned.Name + "!"})
			return
		}
	}
}
