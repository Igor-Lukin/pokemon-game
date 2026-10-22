package api

import mathrand "math/rand"

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

// abilityHeal — восстановить HP носителю (для Аккумулятор/Дренаж), с клампом.
func abilityHeal(p *BattlePokemon, amount, round int, log *[]BattleLogEntry) {
	if amount < 1 {
		amount = 1
	}
	p.CurrentHP += amount
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " поглощает атаку и восстанавливает здоровье!",
	})
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
		setBattleWeather(bs, 2, log)
	case "drizzle": // Морось -> Дождь
		setBattleWeather(bs, 3, log)
	case "snow_warning": // Снегопад -> Град
		setBattleWeather(bs, 4, log)
	case "intimidate": // [batch3] Яростный взгляд: Атака соперника −1
		if opp := abilityOpponentOf(bs, p); opp != nil && !opp.IsFainted {
			if !abilityBlocksStatDrop(opp, 1) {
				*log = append(*log, BattleLogEntry{
					Round: bs.Turn, PokemonName: p.Name, PokemonImage: p.ImageURL,
					Message: p.Name + " устрашает противника!",
				})
				abilityBumpStat(opp, 1, -1, bs.Turn, log)
			}
		}
	}
}

// abilityOnEndTurn — [batch3] конец хода: Форсаж (+скорость), погодное лечение
// (Дождефаг/Ледяное тело), Змеиная кожа (шанс снять статус). Пропускает павших.
func abilityOnEndTurn(bs *BattleState, p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted {
		return
	}
	switch p.AbilityKey {
	case "speed_boost": // Форсаж: Скорость +1
		abilityBumpStat(p, 3, 1, round, log)
	case "rain_dish": // Дождефаг: в Дождь лечит 1/16
		if bs.BattleWeatherID == 3 {
			abilityHeal(p, p.MaxHP/16, round, log)
		}
	case "ice_body": // Ледяное тело: в Град лечит 1/16
		if bs.BattleWeatherID == 4 {
			abilityHeal(p, p.MaxHP/16, round, log)
		}
	case "shed_skin": // Змеиная кожа: 35% снять основной статус
		if p.StatusID > 1 && mathrand.Intn(100) < 35 {
			p.StatusID = 1
			p.SleepTurns = 0
			p.PoisonTurns = 0
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
				Message: p.Name + " сбрасывает кожу и избавляется от статуса!",
			})
		}
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
	return false
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
	return mult
}

// abilityAbsorbsMove — [batch2] ПОГЛОЩЕНИЕ по типу: цель невосприимчива к типу атаки
// (урон не наносится) и получает эффект (лечение/буст/стат). Возвращает true, если
// атака поглощена (движок прекращает нанесение урона). Только для урон-атак.
func abilityAbsorbsMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil || move == nil {
		return false
	}
	immune := func(msg string) {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
			Message: msg,
		})
	}
	switch defender.AbilityKey {
	case "flash_fire": // Внутренний огонь: огонь -> иммунитет + огненные атаки ×1.5
		if move.TypeID == 2 {
			defender.FlashFireBoosted = true
			immune(defender.Name + " поглощает огонь — его огненные атаки усилены!")
			return true
		}
	case "volt_absorb": // Аккумулятор: электро -> иммунитет + лечение ¼
		if move.TypeID == 4 {
			abilityHeal(defender, defender.MaxHP/4, round, log)
			return true
		}
	case "water_absorb": // Дренаж: вода -> иммунитет + лечение ¼
		if move.TypeID == 3 {
			abilityHeal(defender, defender.MaxHP/4, round, log)
			return true
		}
	case "motor_drive": // Генератор: электро -> иммунитет + Скорость+1
		if move.TypeID == 4 {
			immune(defender.Name + " поглощает электричество!")
			abilityBumpStat(defender, 3, 1, round, log)
			return true
		}
	case "sap_sipper": // Травоядность: трава -> иммунитет + Атака+1
		if move.TypeID == 5 {
			immune(defender.Name + " поглощает травяную атаку!")
			abilityBumpStat(defender, 1, 1, round, log)
			return true
		}
	case "storm_drain": // Пористое тело: вода -> иммунитет + СпАтака+1
		if move.TypeID == 3 {
			immune(defender.Name + " поглощает воду!")
			abilityBumpStat(defender, 4, 1, round, log)
			return true
		}
	}
	return false
}

// abilityOnHitByMove — [batch2] ОТВЕТ ПРИ КОНТАКТЕ: цель (defender, носитель
// способности) отвечает атакующему (attacker) после контактной атаки. Вызывается
// только при hit && move.Contact. Статусы гейтятся иммунитетами атакующего.
func abilityOnHitByMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if attacker == nil || defender == nil || attacker.IsFainted {
		return
	}
	contactDamage := func(frac int, word string) {
		d := attacker.MaxHP / frac
		if d < 1 {
			d = 1
		}
		attacker.CurrentHP -= d
		if attacker.CurrentHP < 0 {
			attacker.CurrentHP = 0
		}
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " ранен об " + word + "!",
		})
		if attacker.CurrentHP <= 0 {
			attacker.IsFainted = true
		}
	}
	contactStatus := func(statusID int) {
		if mathrand.Intn(100) < 30 && !abilityBlocksStatus(bs, attacker, statusID) {
			inflictMajorStatus(attacker, statusID, round, log)
		}
	}
	switch defender.AbilityKey {
	case "rough_skin": // Грубая кожа: −⅛ HP атакующему
		contactDamage(8, "грубую кожу")
	case "iron_barbs": // Стальные шипы: −⅛ HP атакующему
		contactDamage(8, "стальные шипы")
	case "static": // Статическое напряжение: 30% паралич
		contactStatus(2)
	case "flame_body": // Горящее тело: 30% ожог
		contactStatus(3)
	case "poison_point": // Ядовитая кожа: 30% отравление
		contactStatus(6)
	case "gooey": // Липкая слизь: Скорость атакующего −1
		abilityBumpStat(attacker, 3, -1, round, log)
	case "cute_charm": // Привлекательность: 30% влюблённость
		if mathrand.Intn(100) < 30 {
			attacker.Infatuated = true
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
				Message: attacker.Name + " очарован!",
			})
		}
	}
}

// abilityModifyOutgoingDamage — множитель ИСХОДЯЩЕГО урона атакующего. Пусто => 1.0.
func abilityModifyOutgoingDamage(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove) float64 {
	if attacker == nil || move == nil {
		return 1.0
	}
	switch attacker.AbilityKey {
	case "swarm": // насекомое ×1.5 в пинче
		if move.TypeID == 12 && abilityInPinch(attacker) {
			return 1.5
		}
	case "overgrow": // трава ×1.5 в пинче
		if move.TypeID == 5 && abilityInPinch(attacker) {
			return 1.5
		}
	case "blaze": // огонь ×1.5 в пинче
		if move.TypeID == 2 && abilityInPinch(attacker) {
			return 1.5
		}
	case "torrent": // вода ×1.5 в пинче
		if move.TypeID == 3 && abilityInPinch(attacker) {
			return 1.5
		}
	case "pure_power", "huge_power": // физ. урон ×2
		if move.CategoryID == 1 {
			return 2.0
		}
	case "reckless": // атаки с отдачей ×1.2
		if move.Drain < 0 {
			return 1.2
		}
	case "flash_fire": // [batch2] огненные атаки ×1.5, если ранее поглотил огонь
		if move.TypeID == 2 && attacker.FlashFireBoosted {
			return 1.5
		}
	case "tough_claws": // [batch2] Мощные когти: контактные атаки ×1.3
		if move.Contact {
			return 1.3
		}
	}
	return 1.0
}

// abilityModifyIncomingDamage — множитель ВХОДЯЩЕГО урона у защищающегося. Пусто => 1.0.
func abilityModifyIncomingDamage(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove) float64 {
	if defender == nil || move == nil {
		return 1.0
	}
	switch defender.AbilityKey {
	case "thick_fat": // ½ урона от огня и льда
		if move.TypeID == 2 || move.TypeID == 6 {
			return 0.5
		}
	case "fur_coat": // ½ урона от физических атак
		if move.CategoryID == 1 {
			return 0.5
		}
	case "fluffy": // [batch2] Пушистик: ½ от контактных физ., ×2 от огня (перемножаются)
		mult := 1.0
		if move.Contact && move.CategoryID == 1 {
			mult *= 0.5
		}
		if move.TypeID == 2 {
			mult *= 2.0
		}
		return mult
	case "filter": // [batch3] Фильтрация/Монолит/Рефрактор: ¾ по (супер)эффективным
		if getTypeEffectiveness(move.TypeID, getPokemonTypes(defender)) > 1.0 {
			return 0.75
		}
	}
	return 1.0
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
	}
	return false
}
