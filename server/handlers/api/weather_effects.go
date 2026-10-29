package api

import "strconv"

// weather_effects.go — слой эффектов погоды: урон по ходам (град/буря),
// погодо-зависимое лечение, а также точность и тип/мощность отдельных атак.
// Движок остаётся тонким: боевые функции дёргают эти хелперы, про конкретную
// погоду/атаку не зная. Привязка атак — маленькими картами по id (набор
// фиксированный, ~10 атак). Уникальные механики = код (принцип HANDOFF).
//
// ID погоды:  1 обычная, 2 солнце, 3 дождь, 4 град, 5 песчаная буря.
// ID типов:   2 Огонь, 3 Вода, 6 Лёд, 9 Земля, 13 Камень, 17 Сталь.

const (
	weatherClear = 1
	weatherSun   = 2
	weatherRain  = 3
	weatherHail  = 4
	weatherSand  = 5
)

// pokemonHasType — есть ли у покемона указанный тип (основной или доп.).
func pokemonHasType(p *BattlePokemon, typeID int) bool {
	for _, t := range getPokemonTypes(p) {
		if t == typeID {
			return true
		}
	}
	return false
}

// applyWeatherChip — урон погоды в конце хода: град и песчаная буря снимают
// по 1/16 макс. HP с каждого покемона, не имеющего иммунитета по типу.
//
//	Град — бьёт всех, кроме Лёд(6).
//	Буря — бьёт всех, кроме Камень(13)/Земля(9)/Сталь(17).
//
// Вызывается один раз за ход ДО postProcessWeather (пока погода ещё активна),
// чтобы обморок от погоды подхватился штатной логикой победы/поражения.
func applyWeatherChip(bs *BattleState, a, b *BattlePokemon, log *[]BattleLogEntry) {
	// [batch7] Штиль/Микроклимат отменяют погодный урон.
	if abilityWeatherNegated(bs) {
		return
	}
	weatherChipOne(bs.BattleWeatherID, bs.Turn, a, log)
	weatherChipOne(bs.BattleWeatherID, bs.Turn, b, log)
}

func weatherChipOne(weatherID, round int, p *BattlePokemon, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted {
		return
	}
	// [batch7] Магический щит блокирует погодный урон.
	if abilityBlocksIndirect(p) {
		return
	}
	var immune bool
	var src string
	switch weatherID {
	case weatherHail:
		immune = pokemonHasType(p, 6) || p.AbilityKey == "snow_cloak" || p.AbilityKey == "overcoat" // Лёд / Белотелый / Укрытие
		src = "града"
	case weatherSand:
		immune = pokemonHasType(p, 13) || pokemonHasType(p, 9) || pokemonHasType(p, 17) ||
			p.AbilityKey == "sand_veil" || p.AbilityKey == "overcoat" // + Пескоструй / Укрытие
		src = "песчаной бури"
	default:
		return
	}
	if immune {
		// [ability log] Пескоструй/Белотелый не только повышают уклонение, но и
		// полностью игнорируют урон своей погоды.
		if p.AbilityKey == "sand_veil" || p.AbilityKey == "snow_cloak" || p.AbilityKey == "overcoat" {
			abilityLog(p, round, log, "weather_immune", "не получает урон от "+src)
		}
		return
	}
	dmg := p.MaxHP / 16
	if dmg < 1 {
		dmg = 1
	}
	p.CurrentHP -= dmg
	if p.CurrentHP < 0 {
		p.CurrentHP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  p.Name,
		PokemonImage: p.ImageURL,
		Message:      p.Name + " страдает от " + src + "! -" + strconv.Itoa(dmg) + " HP",
	})
	if p.CurrentHP <= 0 {
		p.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round:       round,
			PokemonName: p.Name,
			Fainted:     true,
		})
	}
}

// weatherHealPercent — % лечения от макс. HP для погодо-зависимых лечащих атак.
// Возвращает (процент, true) для таких атак; (0, false) — если атака обычная
// (тогда используется её собственный move.HealPercent).
//
//	Фотосинтез(19) / Лунный свет(493) / Рассветный луч(525):
//	     солнце 66% / ясно 50% / прочая погода 25%.
//	Укрепление(542): песчаная буря 66% / иначе 50%.
func weatherHealPercent(moveID, weatherID int) (int, bool) {
	switch moveID {
	case 19, 493, 525:
		switch weatherID {
		case weatherSun:
			return 66, true
		case weatherClear:
			return 50, true
		default:
			return 25, true
		}
	case 542:
		if weatherID == weatherSand {
			return 66, true
		}
		return 50, true
	}
	return 0, false
}

// weatherAdjustMove — вернуть версию хода с погодными правками точности и
// типа/мощности. Для обычных атак возвращает исходный указатель без изменений;
// для погодных — КОПИЮ (сам сохранённый ход не мутируем, погода пересчитывается
// при каждом применении).
//
//	Гроза(238) / Ураган(340): точность дождь→100, солнце→50, иначе базовая.
//	Метель(273):              точность град→100 (не мажет), иначе базовая.
//	Метеозонд(274):           тип по погоде (солнце→Огонь, дождь→Вода,
//	                          град→Лёд, буря→Камень, ясно→Нормальный);
//	                          мощность 100 в любую погоду, кроме обычной (50).
func weatherAdjustMove(move *BattleMove, weatherID int) *BattleMove {
	switch move.ID {
	case 238, 340: // Гроза, Ураган
		m := *move
		if weatherID == weatherRain {
			m.Accuracy = 100
		} else if weatherID == weatherSun {
			m.Accuracy = 50
		}
		return &m
	case 273: // Метель
		m := *move
		if weatherID == weatherHail {
			m.Accuracy = 100
		}
		return &m
	case 274: // Метеозонд (Weather Ball)
		m := *move
		switch weatherID {
		case weatherSun:
			m.TypeID = 2 // Огонь
		case weatherRain:
			m.TypeID = 3 // Вода
		case weatherHail:
			m.TypeID = 6 // Лёд
		case weatherSand:
			m.TypeID = 13 // Камень
		default:
			m.TypeID = 1 // Нормальный
		}
		if weatherID != weatherClear {
			m.Power = 100
		}
		return &m
	}
	return move
}
