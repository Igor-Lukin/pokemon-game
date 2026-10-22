package api

import "strconv"

// hazards.go — ловушки на поле («вторая стопка» полевой стопки поверх SideState).
// В отличие от экранов, ловушки:
//   - ставятся на сторону ПРОТИВНИКА (чтобы бить его входящих покемонов);
//   - НЕ имеют таймера — держатся весь бой (снос — отдельным шагом: Чистое
//     небо / Быстрое вращение);
//   - стакаются слоями (Шипы 1..3, Отравленные шипы 1..2);
//   - срабатывают в момент ВХОДА покемона на сторону (applyEntryHazards),
//     а не по ходам.
//
// Реализованы: Каменная ловушка(480), Шипы(546), Отравленные шипы(505),
// Клейкая паутина(483). Все статусные (кат.3, power 0).
//
// НЮАНС PvE (одиночный дикий): дикий не меняет покемона, поэтому ловушки,
// поставленные ИГРОКОМ на сторону дикого, не срабатывают. Работают ловушки,
// которые ДИКИЙ ставит на сторону игрока, — они бьют входящего при смене.

const (
	moveStealthRock      = 480 // Каменная ловушка
	moveSpikes           = 546 // Шипы
	moveToxicSpikes      = 505 // Отравленные шипы
	moveStickyWeb        = 483 // Клейкая паутина
	spikesMaxLayers      = 3
	toxicSpikesMaxLayers = 2

	typeFlying = 10 // не наземные — иммунны к наземным ловушкам
	typePoison = 8  // Яд — иммунен к отравлению (Отравленные шипы)
	typeSteel  = 17 // Сталь — иммунна к отравлению (Отравленные шипы)
	typeRock   = 13 // тип Каменной ловушки (для расчёта эффективности)
)

// applyHazardMove — если ход это ловушка, ставит её на сторону ПРОТИВНИКА
// (defender). Возвращает true, если ход обработан как ловушка. Слоистые
// ловушки добавляют слой до максимума; при переполнении — «не сработало».
func applyHazardMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, log *[]BattleLogEntry) bool {
	if defender == nil {
		return false
	}
	foe := sideFor(bs, defender) // сторона противника — туда кладём ловушку
	var msg string
	switch move.ID {
	case moveStealthRock:
		if foe.StealthRock {
			msg = "Но камни уже разбросаны — не сработало!"
		} else {
			foe.StealthRock = true
			msg = "Острые камни окружили сторону противника!"
		}
	case moveSpikes:
		if foe.SpikesLayers >= spikesMaxLayers {
			msg = "Но шипов уже максимум — не сработало!"
		} else {
			foe.SpikesLayers++
			msg = "Шипы разбросаны у ног противника (слоёв: " + strconv.Itoa(foe.SpikesLayers) + ")!"
		}
	case moveToxicSpikes:
		if foe.ToxicSpikesLayers >= toxicSpikesMaxLayers {
			msg = "Но отравленных шипов уже максимум — не сработало!"
		} else {
			foe.ToxicSpikesLayers++
			msg = "Отравленные шипы разбросаны у противника (слоёв: " + strconv.Itoa(foe.ToxicSpikesLayers) + ")!"
		}
	case moveStickyWeb:
		if foe.StickyWeb {
			msg = "Но паутина уже раскинута — не сработало!"
		} else {
			foe.StickyWeb = true
			msg = "Липкая паутина раскинута на стороне противника!"
		}
	default:
		return false
	}
	*log = append(*log, BattleLogEntry{
		Round:        bs.Turn,
		PokemonName:  attacker.Name,
		PokemonImage: attacker.ImageURL,
		Message:      msg,
	})
	return true
}

// applyEntryHazards — покемон p входит на сторону side: применяем к нему все
// ловушки этой стороны. Порядок: урон (Каменная ловушка -> Шипы), затем яд
// (Отравленные шипы), затем Скорость (Клейкая паутина). Возвращает true, если
// покемон потерял сознание от ловушек. Летающие (тип 10) иммунны ко всем, кроме
// Каменной ловушки; она бьёт всех с учётом эффективности Камня по типам.
func applyEntryHazards(bs *BattleState, p *BattlePokemon, side *SideState, round int, log *[]BattleLogEntry) bool {
	if p == nil || p.IsFainted {
		return false
	}
	// [ability hook] Способность при ВЫХОДЕ покемона на поле. Единый choke-point всех
	// входов после старта: смена игрока (SwitchHandler/switch-ход), смена/замена врага,
	// вход следующего врага после обморока (handleEnemyFaint). Пусто => no-op (регрессия).
	abilityOnSwitchIn(bs, p, log)
	types := getPokemonTypes(p)
	grounded := !hasTypeID(types, typeFlying)

	// Каменная ловушка: 1/8 макс. HP × эффективность Камня по типам входящего.
	if side.StealthRock {
		eff := getTypeEffectiveness(typeRock, types)
		dmg := int(float64(p.MaxHP) * eff / 8.0)
		if dmg < 1 {
			dmg = 1
		}
		if hazardDamage(p, dmg, round, "острые камни", log) {
			return true
		}
	}

	// Шипы: только наземным. Слои 1/2/3 -> 1/8, 1/6, 1/4 макс. HP.
	if side.SpikesLayers > 0 && grounded {
		den := map[int]float64{1: 8.0, 2: 6.0, 3: 4.0}[side.SpikesLayers]
		dmg := int(float64(p.MaxHP) / den)
		if dmg < 1 {
			dmg = 1
		}
		if hazardDamage(p, dmg, round, "шипы", log) {
			return true
		}
	}

	// Отравленные шипы: наземным. Сталь и Яд иммунны к отравлению — статус не
	// накладывается, шипы остаются лежать. Иначе 1 слой -> яд(6), 2 -> сильный(7).
	if side.ToxicSpikesLayers > 0 && grounded {
		if hasTypeID(types, typeSteel) || hasTypeID(types, typePoison) {
			*log = append(*log, BattleLogEntry{
				Round:        round,
				PokemonName:  p.Name,
				PokemonImage: p.ImageURL,
				Message:      p.Name + " иммунен к яду — отравленные шипы не действуют.",
			})
		} else {
			statusID := 6 // яд
			if side.ToxicSpikesLayers >= 2 {
				statusID = 7 // сильный яд
			}
			inflictMajorStatus(p, statusID, round, log)
		}
	}

	// Клейкая паутина: наземному входящему Скорость -1.
	if side.StickyWeb && grounded {
		if p.StatStages[3] > -6 {
			p.StatStages[3]--
		}
		*log = append(*log, BattleLogEntry{
			Round:        round,
			PokemonName:  p.Name,
			PokemonImage: p.ImageURL,
			Message:      p.Name + " попал в липкую паутину — Скорость снижена!",
		})
	}

	return p.IsFainted
}

// hazardDamage — снять dmg HP с входящего покемона, залогировать источник и
// возможный обморок. Возвращает true, если покемон потерял сознание.
func hazardDamage(p *BattlePokemon, dmg, round int, src string, log *[]BattleLogEntry) bool {
	p.CurrentHP -= dmg
	if p.CurrentHP < 0 {
		p.CurrentHP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  p.Name,
		PokemonImage: p.ImageURL,
		Message:      p.Name + " пострадал от " + src + " при входе (" + strconv.Itoa(dmg) + ")!",
	})
	if p.CurrentHP <= 0 {
		p.IsFainted = true
		return true
	}
	return false
}

// hasTypeID — есть ли typeID в списке типов.
func hasTypeID(types []int, typeID int) bool {
	for _, t := range types {
		if t == typeID {
			return true
		}
	}
	return false
}
