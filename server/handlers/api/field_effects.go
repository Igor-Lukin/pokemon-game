package api

// field_effects.go — общеполевые/сторонние таймерные эффекты (5 ходов), кроме
// экранов (screens.go) и Увлажнения (fire_effects.go):
//   408 Осушение (Mud Sport)    — сила Электричества ×0.33 на поле (как Увлажнение).
//   400 Гравитация (Gravity)    — Летающие теряют иммунитет к Земле (bs.GravityTurns).
//   495 Магнетизм (Magnet Rise) — носитель невосприимчив к Земле (per-pokemon).
//   499 Наговор (Lucky Chant)   — по своей стороне нельзя нанести крит.
// Постановка — в applySideMove (screens.go). Отсчёт — в tickSideTimers/tickOneSide.
// Игровые эффекты — в calculateDamage (Гравитация/Магнетизм/Наговор) и через
// mudSportAdjust (Осушение).

const (
	typeGround        = 9   // тип Земля
	typeElectric      = 4   // тип Электричество
	moveMudSport      = 408 // Осушение
	moveGravity       = 400 // Гравитация
	moveMagnetRise    = 495 // Магнетизм
	moveLuckyChant    = 499 // Наговор
	moveElectrify     = 547 // Электризация
	moveLevitate      = 490 // Левитация (Telekinesis)
	fieldEffectDuration = 5 // раундов действия
	levitateDuration    = 3 // Левитация держится 3 хода (канон)
)

// handleLevitate — Левитация (490, Telekinesis): цель поднимается в воздух —
// невосприимчива к Земле, но по ней ВСЕГДА попадают (не может уклоняться) N ходов.
func handleLevitate(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if move.ID != moveLevitate {
		return false
	}
	if defender == nil {
		return true
	}
	defender.LevitatedTurns = levitateDuration
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
		Message: defender.Name + " поднят в воздух — иммунен к Земле, но не может уклоняться!",
	})
	return true
}

// tickLevitate — обратный отсчёт Левитации у покемона (per-pokemon таймер).
func tickLevitate(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.LevitatedTurns <= 0 {
		return
	}
	p.LevitatedTurns--
	if p.LevitatedTurns == 0 {
		*log = append(*log, BattleLogEntry{Round: round, Message: p.Name + " опускается на землю."})
	}
}

// handleElectrify — Электризация: если ход опередит атаку противника (т.е. цель
// ещё не ходила в этот ход), её следующая атака станет Электрической. Флаг ставим
// безусловно — эффект сработает, только если цель бьёт ПОСЛЕ (иначе гасится в
// конце хода в tickSideTimers). true, если ход обработан.
func handleElectrify(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if move.ID != moveElectrify {
		return false
	}
	if defender == nil {
		return true
	}
	defender.Electrified = true
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
		Message: "Электрическая волна окутала " + defender.Name + " — его атака станет Электрической!",
	})
	return true
}

func fieldLog(p *BattlePokemon, msg string, round int, log *[]BattleLogEntry) {
	*log = append(*log, BattleLogEntry{Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL, Message: msg})
}

// tickGlobalFieldEffects — обратный отсчёт общеполевых Осушения и Гравитации в
// конце хода (в раунд установки не тикаем — приём *SetTurn, как у Увлажнения).
func tickGlobalFieldEffects(bs *BattleState, log *[]BattleLogEntry) {
	if bs.MudSportTurns > 0 && bs.Turn != bs.MudSportSetTurn {
		bs.MudSportTurns--
		if bs.MudSportTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Электростатика поля рассеялась — Электричество снова в полную силу."})
		}
	}
	if bs.GravityTurns > 0 && bs.Turn != bs.GravitySetTurn {
		bs.GravityTurns--
		if bs.GravityTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Гравитация вернулась в норму."})
		}
	}
}

// tickMagnetRise — обратный отсчёт Магнетизма у покемона (per-pokemon таймер).
func tickMagnetRise(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.MagnetRiseTurns <= 0 {
		return
	}
	p.MagnetRiseTurns--
	if p.MagnetRiseTurns == 0 {
		*log = append(*log, BattleLogEntry{Round: round, Message: p.Name + " опускается — Магнетизм иссяк."})
	}
}

// mudSportAdjust — если Осушение активно и ход электрического типа, возвращает
// КОПИЮ хода с мощностью ×0.33 (зеркало waterSportAdjust для Огня).
func mudSportAdjust(move *BattleMove, active bool) *BattleMove {
	if !active || move.TypeID != typeElectric || move.Power <= 0 {
		return move
	}
	m := *move
	m.Power = m.Power * 33 / 100
	if m.Power < 1 {
		m.Power = 1
	}
	return &m
}
