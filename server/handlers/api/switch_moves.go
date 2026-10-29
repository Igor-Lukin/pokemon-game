package api

import mathrand "math/rand"

// switch_moves.go — ходы «ударил -> сменился»: Подставной ход(209, U-turn) и
// Разнонаправленный ток(311, Volt Switch). Сам удар идёт обычной урон-веткой в
// processTurn; смена выполняется ПОСЛЕ удара в MoveHandler (модель «удар потом
// замена»). Дикий в этот ход бил по прежнему покемону — по новому не бьёт.
//
// Здесь же ФАЗИНГ (Рык/Вихрь): выгнать активного покемона противника, на его
// место — случайный другой из команды. В PvE осмысленно только когда дикий
// выгоняет игрока: у дикого команды нет, поэтому против дикого — провал, как и
// если у цели остался последний/единственный покемон.

// phazeMoves — ходы принудительной смены противника.
//
//	30 Рык, 372 Вихрь — чисто статусные (вызов из статусной ветки processTurn);
//	177 Неистовый бросок — урон + фазинг (вызов из executeDamagingMove после удара).
//
// Диспетчеризуются по id в handlePhazeMove (не через effect_status_id). Наборы
// не пересекаются по маршруту: 30/372 (power 0) идут статусной веткой, 177 —
// урон-веткой, поэтому двойного вызова нет.
var phazeMoves = map[int]bool{
	30:  true, // Рык
	177: true, // Неистовый бросок (Circle Throw) — после урона
	372: true, // Вихрь
}

// handlePhazeMove — если move это Рык/Вихрь, выгоняет активного покемона
// противника (defender) и выводит случайного другого в сознании. Провал:
//   - defender — дикий (bs.WildPokemon): у дикого нет команды на замену;
//   - у defender не осталось других покемонов (последний/единственный).
//
// Против игрока: приоритет хода -6, поэтому игрок уже отходил в этот ход.
func handlePhazeMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, log *[]BattleLogEntry) {
	if !phazeMoves[move.ID] {
		return
	}
	// [batch7] Присоски/Стойкость/Хвост-пропеллер не дают выгнать с поля.
	if abilityKeyIn(defender, "suction_cups", "anchor") {
		abilityLog(defender, bs.Turn, log, "", "удерживается на поле — выгнать нельзя")
		return
	}
	if defender == bs.WildPokemon {
		*log = append(*log, BattleLogEntry{
			Round:   bs.Turn,
			Message: "Но на дикого монстра это не действует!",
		})
		return
	}
	// Цель — активный покемон игрока. Кандидаты на выход — прочие в сознании.
	candidates := make([]int, 0, len(bs.PlayerTeam))
	for i := range bs.PlayerTeam {
		if i != bs.ActiveIdx && !bs.PlayerTeam[i].IsFainted {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		*log = append(*log, BattleLogEntry{
			Round:   bs.Turn,
			Message: "Но у " + defender.Name + " нет других монстров — не сработало!",
		})
		return
	}
	*log = append(*log, BattleLogEntry{
		Round:       bs.Turn,
		PokemonName: defender.Name,
		Message:     defender.Name + " выгнан(а) с поля боя!",
	})
	pick := candidates[mathrand.Intn(len(candidates))]
	// performInBattleSwitch сбрасывает волатили уходящего и бьёт входящего
	// ловушками; смена принудительна (связывание/запрет смены игнорируются).
	performInBattleSwitch(bs, bs.PlayerTeam[pick].ID, log)
}

// isSwitchMove — ход «ударил -> сменился» (Подставной ход / Разнонаправленный ток).
func isSwitchMove(moveID int) bool {
	return moveID == 209 || moveID == 311
}

const (
	moveBatonPass    = 551 // Эстафета — смена с переносом бустов статов
	moveTeleport     = 536 // Телепорт — завершить бой с диким (run_success)
	moveHelpingHand  = 443 // Рука помощи — +1 Атака выбранному в команде, -1 Защита себе
	moveAromaticMist = 423 // Ароматный шлейф — +1 СпЗащита выбранному в команде
)

// handleAromaticMist — Ароматный шлейф: +1 Спец.Защита выбранному покемону команды.
func handleAromaticMist(bs *BattleState, switchTo int, round int, log *[]BattleLogEntry) {
	user := &bs.PlayerTeam[bs.ActiveIdx]
	var target *BattlePokemon
	for i := range bs.PlayerTeam {
		if bs.PlayerTeam[i].ID == switchTo && !bs.PlayerTeam[i].IsFainted {
			target = &bs.PlayerTeam[i]
			break
		}
	}
	if target == nil {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: user.Name, MoveFailed: "Ароматный шлейф: некому помочь!"})
		return
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: user.Name, PokemonImage: user.ImageURL,
		Message: user.Name + " окутывает " + target.Name + " ароматным шлейфом!",
	})
	curseBump(target, 5, 1, round, log) // Спец.Защита цели +1
}

// handleHelpingHand — Рука помощи: +1 Атака выбранному покемону команды (switchTo),
// -1 Защита применившему (активному). Смены НЕ происходит.
func handleHelpingHand(bs *BattleState, switchTo int, round int, log *[]BattleLogEntry) {
	user := &bs.PlayerTeam[bs.ActiveIdx]
	var target *BattlePokemon
	for i := range bs.PlayerTeam {
		if bs.PlayerTeam[i].ID == switchTo && !bs.PlayerTeam[i].IsFainted {
			target = &bs.PlayerTeam[i]
			break
		}
	}
	if target == nil {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: user.Name, MoveFailed: "Рука помощи: некому помочь!"})
		return
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: user.Name, PokemonImage: user.ImageURL,
		Message: user.Name + " протягивает руку помощи " + target.Name + "!",
	})
	curseBump(target, 1, 1, round, log) // Атака цели +1
	curseBump(user, 2, -1, round, log)  // Защита применившего -1
}

// handleBatonPass — Эстафета: сменить активного покемона игрока на switchTo,
// перенеся ЕГО изменения статов (StatStages) на входящего. Уходящий отдаёт бусты
// (обнуляются), входящий получает их поверх ловушек/входа.
func handleBatonPass(bs *BattleState, switchTo int, log *[]BattleLogEntry) bool {
	old := &bs.PlayerTeam[bs.ActiveIdx]
	saved := old.StatStages
	old.StatStages = StatStages{} // бусты уходят на входящего
	if !performInBattleSwitch(bs, switchTo, log) {
		old.StatStages = saved // смена не удалась — вернуть
		return false
	}
	newActive := &bs.PlayerTeam[bs.ActiveIdx]
	newActive.StatStages = saved
	*log = append(*log, BattleLogEntry{
		Round: bs.Turn, PokemonName: newActive.Name, PokemonImage: newActive.ImageURL,
		Message: "Эстафета передана — изменения характеристик сохранены!",
	})
	return true
}

// performInBattleSwitch — сменить активного покемона игрока на userPokemonID
// прямо в текущем ходу (без нового хода и без ответной атаки дикого). Сбрасывает
// боевые волатили уходящего и применяет ловушки на входящего. Возвращает false,
// если сменщик не найден / без сознания / это уже активный.
func performInBattleSwitch(bs *BattleState, userPokemonID int, log *[]BattleLogEntry) bool {
	newIdx := -1
	for i := range bs.PlayerTeam {
		if bs.PlayerTeam[i].ID == userPokemonID && !bs.PlayerTeam[i].IsFainted {
			newIdx = i
			break
		}
	}
	if newIdx < 0 || newIdx == bs.ActiveIdx {
		return false
	}

	// Уходящий покемон теряет боевые волатили (как при обычной смене).
	old := &bs.PlayerTeam[bs.ActiveIdx]
	abilityOnSwitchOut(old, bs.Turn, log) // [batch6] Регенератор
	old.ChargingMoveID = 0
	old.Invulnerable = false
	old.LockedMoveID = 0
	old.LockTurns = 0
	old.Trapped = false
	old.TrapTurns = 0
	old.NoEscape = false
	old.Infatuated = false // Влюблённость (Соблазн) спадает при уходе (gender_moves.go)
	old.Rooted = false
	old.AquaRing = false
	old.WishPending = false
	old.BattleTypes = nil  // смена типа заканчивается при уходе (type_moves.go)
	old.HealBlockTurns = 0 // Запрет регенерации спадает при уходе (heal_moves.go)
	old.SureHit = false    // подготовительные флаги спадают при уходе (focus_moves.go)
	old.SureCrit = false
	old.ChargedElectric = false
	old.Foresighted = false
	old.MiracleEyed = false
	old.DestinyBond = false              // Связь судеб спадает при уходе (sacrifice_moves.go)
	old.Grudge = false                   // Жалостливый взор спадает при уходе (sacrifice_moves.go)
	old.Electrified = false              // Электризация спадает при уходе (field_effects.go)
	old.MagnetRiseTurns = 0              // Магнетизм спадает при уходе (field_effects.go)
	old.LevitatedTurns = 0               // Левитация спадает при уходе (field_effects.go)
	old.ActiveTurns = 0                  // счётчик ходов на поле сбрасывается (Татами Блок)
	old.BideTurns, old.BideDamage = 0, 0 // Выжидание прерывается (counter_moves.go)
	old.YawnTurns = 0                    // дремота Зевка спадает при уходе (status_extra_moves.go)
	resetAbilityForm(old, bs.Turn, log)  // форма по HP откатывается (batch5)
	restoreTransform(old)                // Трансформация откатывается (transform_moves.go)
	restoreBaseStats(old)                // обмен/усреднение статов отменяется (stat_swap_moves.go)
	resetBlockState(old)                 // Овации/Запрет/Подстрекательство снимаются (block_moves.go)

	bs.ActiveIdx = newIdx
	active := &bs.PlayerTeam[bs.ActiveIdx]
	*log = append(*log, BattleLogEntry{
		Round:        bs.Turn,
		PokemonName:  active.Name,
		PokemonImage: active.ImageURL,
		Message:      bs.PlayerName + ": " + active.Name + " вступает в бой!",
	})

	// Исцеляющее желание/Лунный танец: входящий полностью исцеляется (до ловушек).
	applyHealingWish(&bs.PlayerSide, active, bs.Turn, log)

	// Ловушки на стороне игрока бьют входящего (hazards.go).
	if applyEntryHazards(bs, active, &bs.PlayerSide, bs.Turn, log) {
		*log = append(*log, BattleLogEntry{
			Round: bs.Turn, PokemonName: active.Name, Fainted: true,
		})
	}
	return true
}
