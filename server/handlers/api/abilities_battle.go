package api

import (
	mathrand "math/rand"
	"pokemon-game/server/db"
	"strconv"
	"strings"
)

// abilities_battle.go — ДИСПЕТЧЕР эффектов способностей («полки» по effect_key).
//
// Хуки размечены и вызваны из движка боя (точки помечены «// [ability hook]»).
// Способность бойца лежит в BattlePokemon.AbilityKey (= abilities.effect_key):
// у игрока грузится в playerPokemonToBattle, у соперника — транзиентно в
// wildToBattlePokemon. Пустой AbilityKey => все хуки нейтральны (1.0/false/no-op).
//
// Батч 1 (4 базовых хука, без правок движка): пинч-бустеры по типу, ×2 физ. урон,
//   отдача, снижение входящего урона, иммунитеты к статусу, погодники на выходе.
// Батч 2 (2 НОВЫХ хука в executeDamagingMove): поглощение по типу (иммунитет+эффект)
//   и ответ при контакте; плюс добор на существующих хуках (tough_claws, fluffy, flash_fire).
//
// Константы движка: типы Fire=2, Water=3, Electric=4, Grass=5, Ice=6, Bug=12;
// категория physical=1; статусы Паралич=2, Ожог=3, Заморозка=4, Сон=5, Отравление=6,
// Сильное отравление=7; погода Солнце=2, Дождь=3, Град=4; стат-ступени: 1 Атака,
// 2 Защита, 3 Скорость, 4 СпАтака, 5 СпЗащита.

// abilityInPinch — HP носителя ниже трети от максимума («пинч»).
func abilityInPinch(p *BattlePokemon) bool {
	return p.MaxHP > 0 && p.CurrentHP*3 < p.MaxHP
}

// abilityIgnoredBy — [batch6] Подавление способностей (mold_breaker): атакующий
// игнорирует защитные способности цели (поглощение, снижение урона, Стойкость).
func abilityIgnoredBy(attacker *BattlePokemon) bool {
	return attacker != nil && (attacker.AbilityKey == "mold_breaker" || attacker.AbilityKey == "unseen_fist")
}

// abilityIgnoresScreens — [batch6] Преодоление барьеров (infiltrator): атаки и
// статусы атакующего проходят сквозь экраны/Ограждение/Туман.
func abilityIgnoresScreens(p *BattlePokemon) bool {
	return p != nil && p.AbilityKey == "infiltrator"
}

// abilityWeatherSpeedMult — [batch6] погодное удвоение Скорости:
// Водоплавающий в дождь, Хлорофилл в солнце.
func abilityWeatherSpeedMult(bs *BattleState, p *BattlePokemon) float64 {
	if bs == nil || p == nil {
		return 1.0
	}
	switch p.AbilityKey {
	case "swift_swim":
		if bs.BattleWeatherID == 3 {
			return 2.0
		}
	case "chlorophyll":
		if bs.BattleWeatherID == 2 {
			return 2.0
		}
	}
	return 1.0
}

// abilityOnSwitchOut — [batch6] эффекты при УХОДЕ покемона с поля. Регенератор
// восстанавливает 1/3 макс. HP (павшему не помогает).
func abilityOnSwitchOut(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted || p.AbilityKey != "regenerator" || p.CurrentHP >= p.MaxHP {
		return
	}
	heal := p.MaxHP / 3
	if heal < 1 {
		heal = 1
	}
	abilityHealMsg(p, heal, round, log, "покидает поле и восстанавливает 1/3 HP")
}

// abilitySheerForceApplies — [batch6] Грубая сила: урон-атака с дополнительным
// эффектом получает +30%, но сам дополнительный эффект не срабатывает.
// Дополнительным считается любой EffectID урон-атаки, снижение статов ЦЕЛИ
// (в т.ч. гарантированное) и шансовые само-изменения. Гарантированные 100%
// само-бусты (Superpower/Close Combat/Overheat) — не вторичка.
func abilitySheerForceApplies(move *BattleMove) bool {
	if move == nil || move.CategoryID == 3 {
		return false
	}
	if move.EffectID != 0 && move.EffectChance > 0 {
		return true
	}
	for _, sc := range move.StatChanges {
		if sc.Chance <= 0 {
			continue
		}
		if sc.Target != 1 || sc.Chance < 100 {
			return true
		}
	}
	return false
}

// abilitySuppressSecondaryMove — копия атаки без вторичных эффектов (для Грубой
// силы): гарантированные (100%) само-изменения статов сохраняются.
func abilitySuppressSecondaryMove(move *BattleMove) *BattleMove {
	if move == nil {
		return nil
	}
	m := *move
	m.EffectID = 0
	m.EffectChance = 0
	if len(m.StatChanges) > 0 {
		kept := make([]StatChange, 0, len(m.StatChanges))
		for _, sc := range m.StatChanges {
			// Сохраняем только гарантированные само-бусты/само-снижения.
			if sc.Target == 1 && sc.Chance >= 100 {
				kept = append(kept, sc)
			}
		}
		m.StatChanges = kept
	}
	return &m
}

// abilityBlocksPowder — [batch6] Укрытие: порошковые/споровые ходы не действуют.
func abilityBlocksPowder(move *BattleMove) bool {
	if move == nil {
		return false
	}
	switch move.ID {
	case 12, 13, 385, 393, 418, 503, 622, 704: // Sleep/Psn Powder, Stun Spore, Spore, Cotton Spore, Powder, Rage/Magic Powder
		return true
	}
	return false
}

// abilityPressureDrain — [batch6] Величие: цель с pressure заставляет атакующего
// тратить дополнительный PP за применённый ход.
func abilityPressureDrain(attacker, defender *BattlePokemon, move *BattleMove) bool {
	if attacker == nil || defender == nil || move == nil || move.PP <= 0 {
		return false
	}
	if defender.AbilityKey != "pressure" {
		return false
	}
	move.PP--
	if move.PP < 0 {
		move.PP = 0
	}
	return true
}

// abilityLog — единый формат лога способностей: «[способность] Имя: эффект».
// onceKey != "" — писать только ПЕРВЫЙ раз за бой (для пассивных множителей,
// точности/критов и т.п., чтобы не спамить каждый удар). onceKey == "" — писать
// при каждом срабатывании (поглощение, лечение, контактный ответ, конец хода).
func abilityLog(p *BattlePokemon, round int, log *[]BattleLogEntry, onceKey, msg string) {
	if p == nil || log == nil {
		return
	}
	name := p.AbilityName
	if name == "" {
		name = p.AbilityKey
	}
	if name == "" {
		return
	}
	if onceKey != "" {
		if p.abilityLogSeen == nil {
			p.abilityLogSeen = map[string]bool{}
		}
		if p.abilityLogSeen[onceKey] {
			return
		}
		p.abilityLogSeen[onceKey] = true
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: "[способность] " + name + ": " + msg,
	})
}

// abilityMultText — короткое «×1.5» для лога множителей.
func abilityMultText(mult float64) string {
	return "×" + strconv.FormatFloat(mult, 'g', -1, 64)
}

// abilityBumpStat — изменить стат-ступень target (клампы ±6) + лог. Переиспользуется
// способностями, меняющими статы (Генератор/Травоядность/Пористое тело/Липкая слизь).
func abilityBumpStat(target *BattlePokemon, statIdx, delta, round int, log *[]BattleLogEntry) {
	if target == nil || delta == 0 {
		return
	}
	target.StatStages[statIdx] += delta
	if target.StatStages[statIdx] > 6 {
		target.StatStages[statIdx] = 6
	} else if target.StatStages[statIdx] < -6 {
		target.StatStages[statIdx] = -6
	}
	name := statChangeNames[statIdx]
	if name == "" {
		name = "Стат"
	}
	verb := "повышена"
	switch {
	case delta >= 2:
		verb = "резко повышена"
	case delta == -1:
		verb = "снижена"
	case delta <= -2:
		verb = "резко снижена"
	}
	*log = append(*log, BattleLogEntry{Round: round, StatChange: name + " " + target.Name + " " + verb + "!"})
}

// abilityHealMsg — восстановить HP носителю с записью в единый лог способностей.
func abilityHealMsg(p *BattlePokemon, amount, round int, log *[]BattleLogEntry, msg string) {
	if p == nil || p.IsFainted {
		return
	}
	if amount < 1 {
		amount = 1
	}
	p.CurrentHP += amount
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	abilityLog(p, round, log, "", msg)
}

// abilityHeal — восстановить HP носителю (поглощение атаки), с клампом.
func abilityHeal(p *BattlePokemon, amount, round int, log *[]BattleLogEntry) {
	abilityHealMsg(p, amount, round, log, "поглощает атаку и восстанавливает здоровье")
}

// abilityOpponentOf — соперник активного покемона p в 1v1 (для Устрашения и т.п.).
func abilityOpponentOf(bs *BattleState, p *BattlePokemon) *BattlePokemon {
	if bs == nil || p == nil {
		return nil
	}
	if p == bs.WildPokemon {
		if bs.ActiveIdx >= 0 && bs.ActiveIdx < len(bs.PlayerTeam) {
			return &bs.PlayerTeam[bs.ActiveIdx]
		}
		return nil
	}
	return bs.WildPokemon
}

// abilityOnSwitchIn — при выходе покемона в бой. Погодники ставят погоду; Устрашение
// снижает Атаку соперника.
func abilityOnSwitchIn(bs *BattleState, p *BattlePokemon, log *[]BattleLogEntry) {
	if p == nil {
		return
	}
	switch p.AbilityKey {
	case "drought": // Засуха -> Солнце
		abilityLog(p, bs.Turn, log, "", "вызывает солнечную погоду")
		setBattleWeather(bs, 2, log)
	case "drizzle": // Морось -> Дождь
		abilityLog(p, bs.Turn, log, "", "вызывает дождь")
		setBattleWeather(bs, 3, log)
	case "snow_warning": // Снегопад -> Град
		abilityLog(p, bs.Turn, log, "", "вызывает град")
		setBattleWeather(bs, 4, log)
	case "intimidate": // [batch3] Яростный взгляд: Атака соперника −1
		if opp := abilityOpponentOf(bs, p); opp != nil && !opp.IsFainted {
			if !abilityBlocksStatDrop(opp, 1) {
				abilityLog(p, bs.Turn, log, "", "устрашает противника: Атака −1")
				abilityBumpStat(opp, 1, -1, bs.Turn, log)
			}
		}
	case "frisk": // [batch6] Проницательность: определяет предмет противника
		if opp := abilityOpponentOf(bs, p); opp != nil && !opp.IsFainted {
			if opp.HeldItemName != "" {
				abilityLog(p, bs.Turn, log, "", "определяет предмет противника: "+opp.HeldItemName)
			} else {
				abilityLog(p, bs.Turn, log, "", "не находит у противника предмета")
			}
		}
	case "telepathy": // [batch6] Телепатия: сообщает тренеру статусные атаки противника
		if opp := abilityOpponentOf(bs, p); opp != nil && !opp.IsFainted {
			var names []string
			for i := range opp.Moves {
				if opp.Moves[i].CategoryID == 3 {
					names = append(names, opp.Moves[i].Name)
				}
			}
			if len(names) == 0 {
				abilityLog(p, bs.Turn, log, "", "читает мысли: у противника нет статусных атак")
			} else {
				abilityLog(p, bs.Turn, log, "", "читает мысли: "+strings.Join(names, ", "))
			}
		}
	}
	abilityExtraSwitchIn(bs, p, log)
	// [batch5] Формы по HP (Стайность/Сброс брони/Тотем): проверяем и на входе.
	abilityUpdateForm(bs, p, bs.Turn, log)
}

// abilityOnEndTurn — [batch3] конец хода: Форсаж (+скорость), погодное лечение
// (Дождефаг/Ледяное тело), Змеиная кожа (шанс снять статус). Пропускает павших.
// [batch5] погодные мультихуки Солнечной батареи/Сухой кожи и формы по HP.
func abilityOnEndTurn(bs *BattleState, p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted {
		return
	}
	switch p.AbilityKey {
	case "speed_boost": // Форсаж: Скорость +1
		abilityLog(p, round, log, "", "повышает Скорость на 1")
		abilityBumpStat(p, 3, 1, round, log)
	case "rain_dish": // Дождефаг: в Дождь лечит 1/16
		if bs.BattleWeatherID == 3 {
			abilityHealMsg(p, p.MaxHP/16, round, log, "в дождь восстанавливает 1/16 HP")
		}
	case "ice_body": // Ледяное тело: в Град лечит 1/16
		if bs.BattleWeatherID == 4 {
			abilityHealMsg(p, p.MaxHP/16, round, log, "в град восстанавливает 1/16 HP")
		}
	case "shed_skin": // Змеиная кожа: 35% снять основной статус
		if p.StatusID > 1 && mathrand.Intn(100) < 35 {
			p.StatusID = 1
			p.SleepTurns = 0
			p.PoisonTurns = 0
			abilityLog(p, round, log, "", "сбрасывает кожу и избавляется от статуса")
		}
	case "hydration": // [batch6] Впитывание: в дождь снимает основной статус
		if bs.BattleWeatherID == 3 && p.StatusID > 1 {
			p.StatusID = 1
			p.SleepTurns = 0
			p.PoisonTurns = 0
			abilityLog(p, round, log, "", "в дождь смывает статусные проблемы")
		}
	case "solar_power": // [batch5] Солнечная батарея: в солнце теряет 1/8
		if bs.BattleWeatherID == 2 {
			abilityWeatherHP(p, -p.MaxHP/8, "солнце обжигает — −1/8 HP", round, log)
		}
	case "dry_skin": // [batch5] Сухая кожа: дождь лечит 1/8, солнце жжёт 1/8
		switch bs.BattleWeatherID {
		case 3:
			abilityWeatherHP(p, p.MaxHP/8, "впитывает влагу дождя — +1/8 HP", round, log)
		case 2:
			abilityWeatherHP(p, -p.MaxHP/8, "солнце иссушает — −1/8 HP", round, log)
		}
	}
	abilityExtraEndTurn(bs, p, round, log)
	// [batch5] Формы по HP: в конце раунда пересчитать форму.
	abilityUpdateForm(bs, p, round, log)
}

// abilityWeatherHP — изменение HP от погодной способности (±) с логом и
// обработкой обморока. Используется Солнечной батареей/Сухой кожей.
func abilityWeatherHP(p *BattlePokemon, delta int, msg string, round int, log *[]BattleLogEntry) {
	if p == nil || delta == 0 || p.IsFainted {
		return
	}
	if delta < 0 && abilityBlocksIndirect(p) {
		return
	}
	p.CurrentHP += delta
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	if p.CurrentHP < 0 {
		p.CurrentHP = 0
	}
	abilityLog(p, round, log, "", msg)
	if p.CurrentHP <= 0 {
		p.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, Fainted: true,
		})
	}
}

// abilityBlocksStatDrop — [batch3] способность блокирует СНИЖЕНИЕ стата statIdx
// противником (Чистое тело/Белый дым — все статы). Пусто => false.
func abilityBlocksStatDrop(target *BattlePokemon, statIdx int) bool {
	if target == nil {
		return false
	}
	switch target.AbilityKey {
	case "clear_body": // Чистое тело / Белый дым: блок любого снижения статов
		return true
	case "keen_eye": // [batch4] Соколиный глаз: нельзя снизить Точность (6)
		return statIdx == 6
	case "big_pecks": // [batch4] Прочные перья: нельзя снизить Защиту (2)
		return statIdx == 2
	}
	return abilityBlocksStatDropExtra(target, statIdx)
}

// abilityBlocksCrit — [batch4] защищающийся не получает критов (Боевая броня/Крепкий
// панцирь). Пусто => false.
func abilityBlocksCrit(defender *BattlePokemon) bool {
	return defender != nil && defender.AbilityKey == "battle_armor"
}

// abilityIgnoresEvasion — [batch4] атакующий игнорирует повышенное уклонение цели
// (Соколиный глаз). Пусто => false.
func abilityIgnoresEvasion(attacker *BattlePokemon) bool {
	return attacker != nil && attacker.AbilityKey == "keen_eye"
}

// abilityAccuracyMult — [batch4] множитель ТОЧНОСТИ атаки. compound_eyes (Фасеточные
// глаза) повышает точность атакующего ×1.3; bright_body (Блестящая кожа) снижает
// точность СТАТУС-атак по носителю ×0.5. Пусто => 1.0.
func abilityAccuracyMult(attacker, defender *BattlePokemon, move *BattleMove) float64 {
	mult := 1.0
	if attacker != nil && attacker.AbilityKey == "compound_eyes" {
		mult *= 1.3
	}
	if defender != nil && move != nil && move.CategoryID == 3 && defender.AbilityKey == "bright_body" {
		mult *= 0.5
	}
	if attacker != nil && move != nil && move.CategoryID == 1 && attacker.AbilityKey == "hustle" {
		mult *= 0.8 // [batch6] Нетерпеливость: точность физических атак −20%
	}
	mult *= abilityAccuracyMultExtra(attacker, defender, move)
	return mult
}

// abilityAbsorbType — [batch2/5] общее ядро ПОГЛОЩЕНИЯ по типу: цель
// невосприимчива к типу (урон/эффект не применяются) и получает свой бонус
// (лечение/буст/стат). Используется и урон-атаками, и статус-атаками.
func abilityAbsorbType(defender *BattlePokemon, typeID, round int, log *[]BattleLogEntry) bool {
	if defender == nil {
		return false
	}
	switch defender.AbilityKey {
	case "flash_fire": // Внутренний огонь: огонь -> иммунитет + огненные атаки ×1.5
		if typeID == 2 {
			defender.FlashFireBoosted = true
			abilityLog(defender, round, log, "", "поглощает огонь — его огненные атаки усилены")
			return true
		}
	case "volt_absorb": // Аккумулятор: электро -> иммунитет + лечение ¼
		if typeID == 4 {
			abilityHealMsg(defender, defender.MaxHP/4, round, log, "поглощает электричество и восстанавливает 1/4 HP")
			return true
		}
	case "water_absorb": // Дренаж: вода -> иммунитет + лечение ¼
		if typeID == 3 {
			abilityHealMsg(defender, defender.MaxHP/4, round, log, "поглощает воду и восстанавливает 1/4 HP")
			return true
		}
	case "dry_skin": // [batch5] Сухая кожа: вода -> иммунитет + лечение ¼
		if typeID == 3 {
			abilityHealMsg(defender, defender.MaxHP/4, round, log, "впитывает воду и восстанавливает 1/4 HP")
			return true
		}
	case "motor_drive": // Генератор: электро -> иммунитет + Скорость+1
		if typeID == 4 {
			abilityLog(defender, round, log, "", "поглощает электричество — Скорость +1")
			abilityBumpStat(defender, 3, 1, round, log)
			return true
		}
	case "sap_sipper": // Травоядность: трава -> иммунитет + Атака+1
		if typeID == 5 {
			abilityLog(defender, round, log, "", "поглощает травяную атаку — Атака +1")
			abilityBumpStat(defender, 1, 1, round, log)
			return true
		}
	case "storm_drain": // Пористое тело: вода -> иммунитет + СпАтака+1
		if typeID == 3 {
			abilityLog(defender, round, log, "", "поглощает воду — Сп.Атака +1")
			abilityBumpStat(defender, 4, 1, round, log)
			return true
		}
	}
	return false
}

// abilityAbsorbsMove — [batch2] поглощение УРОН-атаки по типу (урон не наносится).
func abilityAbsorbsMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil || move == nil || abilityIgnoredBy(attacker) {
		return false
	}
	if abilityAbsorbsMoveExtra(bs, attacker, defender, move, round, log) {
		return true
	}
	return abilityAbsorbType(defender, move.TypeID, round, log)
}

// abilityAbsorbsStatusMove — [batch5] поглощение СТАТУС-атаки по типу. Не
// трогает self-target и общеполевые погодные ходы (Ясный день/Танец дождя),
// которые лишь меняют погоду, а не бьют по носителю.
func abilityAbsorbsStatusMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil || move == nil || move.CategoryID != 3 || move.TargetID == 2 {
		return false
	}
	if abilityBlocksPriorityMove(defender, move) {
		abilityLog(defender, round, log, "", "блокирует приоритетную атаку")
		return true
	}
	if abilityBlocksStatusMoveExtra(bs, attacker, defender, move, round, log) {
		return true
	}
	// [batch6] Укрытие: порошковые/споровые ходы не действуют.
	if defender.AbilityKey == "overcoat" && abilityBlocksPowder(move) {
		abilityLog(defender, round, log, "", "не пропускает порошок — ход не действует")
		return true
	}
	if move.EffectID >= 14 && move.EffectID <= 17 {
		return false
	}
	if abilityAbsorbsMoveExtra(bs, attacker, defender, move, round, log) {
		return true
	}
	return abilityAbsorbType(defender, move.TypeID, round, log)
}

// abilityOnHitByMove — [batch2] ОТВЕТ ПРИ КОНТАКТЕ: цель (defender, носитель
// способности) отвечает атакующему (attacker) после контактной атаки. Вызывается
// только при hit && move.Contact. Статусы гейтятся иммунитетами атакующего.
func abilityOnHitByMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if attacker == nil || defender == nil || attacker.IsFainted {
		return
	}
	contactDamage := func(frac int, msg string) {
		d := attacker.MaxHP / frac
		if d < 1 {
			d = 1
		}
		attacker.CurrentHP -= d
		if attacker.CurrentHP < 0 {
			attacker.CurrentHP = 0
		}
		abilityLog(defender, round, log, "", msg+" "+attacker.Name+" (−"+strconv.Itoa(d)+" HP)")
		if attacker.CurrentHP <= 0 {
			attacker.IsFainted = true
		}
	}
	contactStatus := func(statusID int) {
		if mathrand.Intn(100) >= 30 {
			return
		}
		if abilityBlocksStatus(bs, attacker, statusID) {
			abilityLog(attacker, round, log, "status_block_"+strconv.Itoa(statusID), "защищает от статуса: "+statusRegistry[statusID].Name)
			return
		}
		abilityLog(defender, round, log, "", "заражает "+attacker.Name+": "+statusRegistry[statusID].Name)
		inflictMajorStatus(attacker, statusID, round, log)
	}
	switch defender.AbilityKey {
	case "rough_skin": // Грубая кожа: −⅛ HP атакующему
		contactDamage(8, "наносит урон")
	case "iron_barbs": // Стальные шипы: −⅛ HP атакующему
		contactDamage(8, "наносит урон шипами")
	case "static": // Статическое напряжение: 30% паралич
		contactStatus(2)
	case "flame_body": // Горящее тело: 30% ожог
		contactStatus(3)
	case "poison_point": // Ядовитая кожа: 30% отравление
		contactStatus(6)
	case "gooey": // Липкая слизь: Скорость атакующего −1
		abilityLog(defender, round, log, "gooey", "снижает Скорость "+attacker.Name+" на 1")
		abilityBumpStat(attacker, 3, -1, round, log)
	case "cute_charm": // Привлекательность: 30% влюблённость
		if attacker.AbilityKey == "oblivious" { // [batch6] Забывчивость
			abilityLog(attacker, round, log, "", "не поддаётся очарованию")
			return
		}
		if mathrand.Intn(100) < 30 {
			attacker.Infatuated = true
			abilityLog(defender, round, log, "", "очаровывает "+attacker.Name)
		}
	}
	abilityExtraContact(bs, attacker, defender, move, round, log)
}

// abilityAfterBeingHit — [batch5] реакция НОСИТЕЛЯ на любой физический удар (не
// только контактный): Хрупкая броня — Защита −1, Скорость +2.
func abilityAfterBeingHit(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if defender == nil || move == nil || defender.IsFainted {
		return
	}
	if defender.AbilityKey == "weak_armor" && move.CategoryID == 1 {
		abilityLog(defender, round, log, "weak_armor", "теряет броню: Защита −1, Скорость +2")
		abilityBumpStat(defender, 2, -1, round, log)
		abilityBumpStat(defender, 3, 2, round, log)
	}
}

// abilityEvasionBonus — [batch5] бонус к ступени уклонения от погоды:
// Пескоструй (sand_veil) +1 в песчаную бурю, Белотелый (snow_cloak) +1 в град.
func abilityEvasionBonus(defender *BattlePokemon, weatherID int) int {
	if defender == nil {
		return 0
	}
	switch defender.AbilityKey {
	case "sand_veil":
		if weatherID == 5 {
			return 1
		}
	case "snow_cloak":
		if weatherID == 4 {
			return 1
		}
	}
	return 0
}

// abilityHovering — [batch5] Левитация: постоянный пар над землёй (иммунитет к
// Земле и наземным ловушкам), кроме действия Гравитации.
func abilityHovering(p *BattlePokemon, bs *BattleState) bool {
	if p == nil || p.AbilityKey != "levitate" {
		return false
	}
	return bs == nil || bs.GravityTurns == 0
}

// abilityModifyOutgoingDamage — множитель ИСХОДЯЩЕГО урона атакующего. Пусто => 1.0.
func abilityModifyOutgoingDamage(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) float64 {
	if attacker == nil || move == nil {
		return 1.0
	}
	out := func(key string, mult float64, msg string) float64 {
		abilityLog(attacker, round, log, key, msg+" "+abilityMultText(mult))
		return mult
	}
	switch attacker.AbilityKey {
	case "swarm": // насекомое ×1.5 в пинче
		if move.TypeID == 12 && abilityInPinch(attacker) {
			return out("swarm", 1.5, "в пинче усиливает атаки Насекомого")
		}
	case "overgrow": // трава ×1.5 в пинче
		if move.TypeID == 5 && abilityInPinch(attacker) {
			return out("overgrow", 1.5, "в пинче усиливает травяные атаки")
		}
	case "blaze": // огонь ×1.5 в пинче
		if move.TypeID == 2 && abilityInPinch(attacker) {
			return out("blaze", 1.5, "в пинче усиливает огненные атаки")
		}
	case "torrent": // вода ×1.5 в пинче
		if move.TypeID == 3 && abilityInPinch(attacker) {
			return out("torrent", 1.5, "в пинче усиливает водные атаки")
		}
	case "pure_power", "huge_power": // физ. урон ×2
		if move.CategoryID == 1 {
			return out("power", 2.0, "удваивает физический урон")
		}
	case "reckless": // атаки с отдачей ×1.2
		if move.Drain < 0 {
			return out("reckless", 1.2, "усиливает атаки с отдачей")
		}
	case "flash_fire": // [batch2] огненные атаки ×1.5, если ранее поглотил огонь
		if move.TypeID == 2 && attacker.FlashFireBoosted {
			return out("flash_fire", 1.5, "усиливает огненные атаки после поглощения")
		}
	case "tough_claws": // [batch2] Мощные когти: контактные атаки ×1.3
		if move.Contact {
			return out("tough_claws", 1.3, "усиливает контактные атаки")
		}
	case "solar_power": // [batch5] Солнечная батарея: спец. атаки ×1.5 в солнце
		if bs != nil && bs.BattleWeatherID == 2 && move.CategoryID == 2 {
			return out("solar_power", 1.5, "в солнце усиливает специальные атаки")
		}
	case "technician": // [batch6] Мастерство: атаки силой ≤60 ×1.5
		if move.Power > 0 && move.Power <= 60 {
			return out("technician", 1.5, "усиливает слабые атаки")
		}
	case "sheer_force": // [batch6] Грубая сила: атаки с доп. эффектом ×1.3
		if abilitySheerForceApplies(move) {
			return out("sheer_force", 1.3, "усиливает атаки с дополнительным эффектом")
		}
	}
	return abilityOutgoingMultExtra(bs, attacker, defender, move, round, log)
}

// abilityModifyIncomingDamage — множитель ВХОДЯЩЕГО урона у защищающегося. Пусто => 1.0.
func abilityModifyIncomingDamage(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) float64 {
	if defender == nil || move == nil || abilityIgnoredBy(attacker) {
		return 1.0
	}
	in := func(key string, mult float64, msg string) float64 {
		abilityLog(defender, round, log, key, msg+" "+abilityMultText(mult))
		return mult
	}
	switch defender.AbilityKey {
	case "thick_fat": // ½ урона от огня и льда
		if move.TypeID == 2 || move.TypeID == 6 {
			return in("thick_fat", 0.5, "снижает урон от Огня/Льда")
		}
	case "fur_coat": // ½ урона от физических атак
		if move.CategoryID == 1 {
			return in("fur_coat", 0.5, "снижает физический урон")
		}
	case "fluffy": // [batch2] Пушистик: ½ от контактных физ., ×2 от огня (перемножаются)
		mult := 1.0
		msg := "изменяет входящий урон"
		if move.Contact && move.CategoryID == 1 {
			mult *= 0.5
			msg = "снижает контактный физический урон"
		}
		if move.TypeID == 2 {
			mult *= 2.0
			msg = "усиливает получаемый урон от Огня"
			if move.Contact && move.CategoryID == 1 {
				msg = "снижает контактный физ. урон, но усиливает Огонь"
			}
		}
		return in("fluffy", mult, msg)
	case "filter": // [batch3] Фильтрация/Монолит/Рефрактор: ¾ по (супер)эффективным
		if getTypeEffectiveness(move.TypeID, getPokemonTypes(defender)) > 1.0 {
			return in("filter", 0.75, "снижает суперэффективный урон")
		}
	case "dry_skin": // [batch5] Сухая кожа: огонь бьёт на 25% сильнее
		if move.TypeID == 2 {
			return in("dry_skin", 1.25, "усиливает получаемый урон от Огня")
		}
	}
	return abilityIncomingMultExtra(bs, attacker, defender, move, round, log)
}

// abilityBlocksStatus — иммунитет цели к статусу от способности. Пусто => false.
// statusID — id накладываемого статуса (2 паралич, 5 сон, 6 яд, 7 сильн. яд, …).
func abilityBlocksStatus(bs *BattleState, target *BattlePokemon, statusID int) bool {
	if target == nil {
		return false
	}
	switch target.AbilityKey {
	case "immunity": // не отравляется (обычный и сильный яд)
		return statusID == 6 || statusID == 7
	case "limber": // не парализуется
		return statusID == 2
	case "insomnia", "vital_spirit": // не усыпляется
		return statusID == 5
	case "leaf_guard": // [batch6] Лиственный щит: в солнце иммунитет к статусам
		return bs != nil && bs.BattleWeatherID == 2 && statusID >= 2 && statusID <= 7
	}
	return false
}

// ---------------------------------------------------------------------------
// ФОРМЫ (Стайность/Сброс брони/Тотем/Королевское построение и аналоги).
// Правила форм — данные в таблице pokemon_forms; движок лишь переключает
// имя/спрайт/типы и множители статов. Таблица может быть пустой — тогда хук no-op.
// ---------------------------------------------------------------------------

// abilityUsesHPForm — способность управляет формой по порогу HP.
func abilityUsesHPForm(key string) bool {
	switch key {
	case "schooling", "shields_down", "totem":
		return true
	}
	return false
}

// abilityUsesMoveForm — способность переключает форму использованным ходом.
func abilityUsesMoveForm(key string) bool {
	return key == "stance_change"
}

// abilityUpdateForm — привести форму к состоянию по текущему HP/уровню.
func abilityUpdateForm(bs *BattleState, p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted {
		return
	}
	if abilityUsesMoveForm(p.AbilityKey) {
		// Форму задаёт последний использованный ход; в конце хода не трогаем.
		return
	}
	if !abilityUsesHPForm(p.AbilityKey) {
		resetAbilityForm(p, round, log)
		return
	}
	hpPercent := 0
	if p.MaxHP > 0 {
		hpPercent = p.CurrentHP * 100 / p.MaxHP
	}
	form, ok := db.FormFor(p.PokemonID, p.AbilityKey, p.Level, hpPercent)
	if !ok {
		resetAbilityForm(p, round, log)
		return
	}
	if p.FormKey == form.FormKey {
		return
	}
	applyAbilityForm(p, form, round, log)
}

// abilityOnMoveUsed — переключение формы перед применением хода. Для
// stance_change: атакующий ход -> боевая форма (blade), Королевский щит ->
// базовая защитная форма. Если правило не найдено, текущая форма сохраняется.
func abilityOnMoveUsed(bs *BattleState, p *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if bs == nil || p == nil || move == nil || p.IsFainted {
		return
	}
	if !abilityUsesMoveForm(p.AbilityKey) {
		return
	}
	form, ok := db.FormForMove(p.PokemonID, p.AbilityKey, move.ID, move.CategoryID, move.Power)
	if !ok {
		return
	}
	if form.IsBaseForm {
		resetAbilityForm(p, round, log)
		return
	}
	if p.FormKey == form.FormKey {
		return
	}
	applyAbilityForm(p, form, round, log)
}

// applyAbilityForm — включить форму (снимок исходных значений при первом входе).
func applyAbilityForm(p *BattlePokemon, f *db.BattleForm, round int, log *[]BattleLogEntry) {
	if p == nil || f == nil {
		return
	}
	if p.FormKey == "" {
		p.formBaseName = p.Name
		p.formBaseImage = p.ImageURL
		p.formBaseTypes = append([]int(nil), getPokemonTypes(p)...)
		p.formBaseAttack = p.Attack
		p.formBaseDefense = p.Defense
		p.formBaseSpeed = p.Speed
		p.formBaseSpecialAttack = p.SpecialAttack
		p.formBaseSpecialDefense = p.SpecialDefense
	}
	p.FormKey = f.FormKey
	if f.FormName != "" {
		p.Name = f.FormName
	}
	if f.ImageURL != "" {
		p.ImageURL = f.ImageURL
	}
	if f.PrimaryTypeID > 0 {
		p.PrimaryTypeID = f.PrimaryTypeID
		if f.SecondaryTypeID > 0 {
			st := f.SecondaryTypeID
			p.SecondaryTypeID = &st
		} else {
			p.SecondaryTypeID = nil
		}
	}
	p.Attack = formStat(p.formBaseAttack, f.AttackMult)
	p.Defense = formStat(p.formBaseDefense, f.DefenseMult)
	p.Speed = formStat(p.formBaseSpeed, f.SpeedMult)
	p.SpecialAttack = formStat(p.formBaseSpecialAttack, f.SpecialAttackMult)
	p.SpecialDefense = formStat(p.formBaseSpecialDefense, f.SpecialDefenseMult)
	abilityLog(p, round, log, "", "принимает другую форму")
}

// resetAbilityForm — вернуть исходную форму (уход с поля / условие пропало).
func resetAbilityForm(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.FormKey == "" {
		return
	}
	p.FormKey = ""
	p.Name = p.formBaseName
	p.ImageURL = p.formBaseImage
	p.PrimaryTypeID = 0
	p.SecondaryTypeID = nil
	if len(p.formBaseTypes) > 0 {
		p.PrimaryTypeID = p.formBaseTypes[0]
		if len(p.formBaseTypes) > 1 {
			st := p.formBaseTypes[1]
			p.SecondaryTypeID = &st
		}
	}
	p.Attack = p.formBaseAttack
	p.Defense = p.formBaseDefense
	p.Speed = p.formBaseSpeed
	p.SpecialAttack = p.formBaseSpecialAttack
	p.SpecialDefense = p.formBaseSpecialDefense
	abilityLog(p, round, log, "", "возвращается в исходную форму")
}

// applySturdy — [batch6] Здоровяк: удар, который должен был отправить в обморок
// покемона с ПОЛНЫМ HP, оставляет 1 HP. ready сбрасывается после срабатывания.
func applySturdy(defender, attacker *BattlePokemon, ready bool, round int, log *[]BattleLogEntry) bool {
	if !ready || defender == nil || defender.IsFainted || defender.CurrentHP > 0 || abilityIgnoredBy(attacker) {
		return false
	}
	defender.CurrentHP = 1
	defender.IsFainted = false
	abilityLog(defender, round, log, "", "выдерживает удар и остаётся с 1 HP")
	return true
}

func formStat(base int, mult float64) int {
	if mult <= 0 {
		mult = 1
	}
	v := int(float64(base)*mult + 0.5)
	if v < 1 {
		v = 1
	}
	return v
}
