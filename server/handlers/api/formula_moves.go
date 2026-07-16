package api

import mathrand "math/rand"

// formula_moves.go — атаки с нестандартной формулой урона. Считаются в
// calculateDamage до/вместо обычной формулы. Наборы фиксированные (по id).
//
// Важно: у части этих атак в БД мощность выставлена ненулевой ТОЛЬКО для
// маршрутизации (движок гонит power==0 в статусную ветку). Реальный урон
// определяется формулами ниже, а не move.Power.

// fixedDamageMoves — ПОСТОЯННЫЙ урон (игнорирует типы/статы/крит; но не
// действует на иммунных по типу цели): id -> величина урона.
var fixedDamageMoves = map[int]int{
	45:  40, // Ярость дракона — всегда 40
	250: 20, // Звуковая волна — всегда 20
}

// halfHPMoves — снимают ПОЛОВИНУ ТЕКУЩЕГО HP цели (Super Fang).
var halfHPMoves = map[int]bool{
	41:  true, // Бешенство
	222: true, // Буйство природы
}

// userHPPowerMoves — мощность = базовая * (тек.HP / макс.HP) пользователя
// (Водный смерч / Извержение): id -> базовая мощность.
var userHPPowerMoves = map[int]int{
	229: 150, // Водный смерч
	254: 150, // Извержение
}

// foeHPPowerMoves — мощность = базовая * (тек.HP / макс.HP) ЦЕЛИ: чем выше HP
// цели, тем сильнее удар (Захват/Wring Out). В БД power=1 — сентинел маршрутизации.
var foeHPPowerMoves = map[int]int{
	248: 120, // Захват — до 120 при полном HP цели
}

// physicalHitSpecialMoves — спец-атаки, бьющие по ФИЗИЧЕСКОЙ Защите цели
// (Психовыстрел / Психошок). Атакующая характеристика — спец-атака.
var physicalHitSpecialMoves = map[int]bool{
	306: true, // Психовыстрел
	308: true, // Психошок
}

// ohkoMoves — атаки мгновенного обморока (One-Hit KO): при попадании цель
// теряет весь HP (урон = макс. HP цели). Точность считается особой формулой
// (см. ohkoAccuracy) в обход модификаторов точности/уклонения.
var ohkoMoves = map[int]bool{
	462: true, // Гильотина
	453: true, // Абсолютный холод
	524: true, // Разлом
	527: true, // Роговое сверло
}

// ohkoAccuracy — шанс попадания OHKO-атаки, %: 30 + 1 за каждый уровень, на
// который пользователь выше цели. Провал (0), если цель выше уровнем.
func ohkoAccuracy(attacker, defender *BattlePokemon) int {
	if attacker.Level < defender.Level {
		return 0
	}
	acc := 30 + (attacker.Level - defender.Level)
	if acc > 100 {
		acc = 100
	}
	return acc
}

// formulaFixedDamage — фиксированный / долевой урон (до обычной формулы).
// Возвращает (урон, эффективность, true), если атака обрабатывается здесь.
// Иммунные по типу цели получают 0 (эффективность 0). Для прочих эффективность
// 1 — у фиксированных атак нет "супер-эффективно/не очень".
func formulaFixedDamage(attacker, defender *BattlePokemon, move *BattleMove) (int, float64, bool) {
	fixed, isFixed := fixedDamageMoves[move.ID]
	isHalf := halfHPMoves[move.ID]
	isOHKO := ohkoMoves[move.ID]
	isFinal := move.ID == 342   // Финальный ход — урон = текущий HP пользователя
	isPsywave := move.ID == 305 // Психическая волна — случайный урон
	isNightShade := move.ID == 280 // Ночная тень — урон = уровень пользователя
	if !isFixed && !isHalf && !isOHKO && !isFinal && !isPsywave && !isNightShade {
		return 0, 0, false
	}
	// Иммунитет по типу — атака не действует.
	if getTypeEffectiveness(move.TypeID, getPokemonTypes(defender)) == 0 {
		return 0, 0, true
	}
	dmg := fixed
	switch {
	case isHalf:
		dmg = defender.CurrentHP / 2
	case isOHKO:
		dmg = defender.MaxHP // мгновенный обморок
	case isFinal:
		dmg = attacker.CurrentHP
	case isPsywave:
		dmg = attacker.Level * (50 + mathrand.Intn(101)) / 100 // 50%..150% уровня
	case isNightShade:
		dmg = attacker.Level // урон точно равен уровню пользователя
	}
	if dmg < 1 {
		dmg = 1
	}
	return dmg, 1, true
}

// sharesType — есть ли у a и b хотя бы один общий тип (для Синхровзрыва).
func sharesType(a, b *BattlePokemon) bool {
	for _, ta := range getPokemonTypes(a) {
		for _, tb := range getPokemonTypes(b) {
			if ta == tb {
				return true
			}
		}
	}
	return false
}

// powerDoubled — удваивается ли мощность атаки при условии на цель:
//   218 Пробуждающий удар — цель спит (StatusID==5);
//   317 Сглаз            — у цели есть мажорный статус (есть в statusRegistry);
//   277 Морская вода     — HP цели <= половины максимального;
//   31  Веношок          — цель отравлена (яд/сильный яд).
func powerDoubled(defender *BattlePokemon, move *BattleMove) bool {
	switch move.ID {
	case 218:
		return defender.StatusID == 5
	case 317:
		_, ok := statusRegistry[defender.StatusID]
		return ok
	case 277:
		return defender.CurrentHP*2 <= defender.MaxHP
	case 74: // Землетрясение по зарытому под землю (Подкоп) — удвоенный урон
		return defender.Invulnerable && defender.ChargingMoveID == 206
	case 31: // Веношок — ×2 по отравленной цели (яд 6 / сильный яд 7)
		return defender.StatusID == 6 || defender.StatusID == 7
	}
	return false
}

// applyTriAttackEffect — Тройная атака(337): при попадании 20% шанс наложить
// СЛУЧАЙНЫЙ мажорный статус из трёх — ожог(3)/заморозка(4)/паралич(2).
// Накладывается, только если у цели ещё нет мажорного статуса.
func applyTriAttackEffect(defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if move.ID != 337 || defender == nil || defender.IsFainted {
		return
	}
	if mathrand.Intn(100) >= 20 {
		return
	}
	statuses := []int{3, 4, 2} // ожог / заморозка / паралич
	inflictMajorStatus(defender, statuses[mathrand.Intn(len(statuses))], round, log)
}

// weightBasedPower — мощность атак, зависящих от веса. (мощность, true) для
// таких атак; (0, false) — иначе.
//   208 Подножка / 336 Травяные путы — по весу ЦЕЛИ (20..120).
//   215 Пресс — по отношению веса пользователя к весу цели (40..120).
func weightBasedPower(attacker, defender *BattlePokemon, move *BattleMove) (int, bool) {
	switch move.ID {
	case 208, 336:
		return targetWeightPower(defender.Weight), true
	case 215:
		return heavySlamPower(attacker.Weight, defender.Weight), true
	}
	return 0, false
}

// targetWeightPower — мощность по весу цели (кг): чем тяжелее, тем сильнее.
func targetWeightPower(w float64) int {
	switch {
	case w < 10:
		return 20
	case w < 25:
		return 40
	case w < 50:
		return 60
	case w < 100:
		return 80
	case w < 200:
		return 100
	default:
		return 120
	}
}

// heavySlamPower — мощность по отношению веса пользователя к весу цели: чем
// пользователь тяжелее относительно цели, тем сильнее (40..120).
func heavySlamPower(att, def float64) int {
	if def <= 0 {
		return 120
	}
	switch r := att / def; {
	case r >= 5:
		return 120
	case r >= 4:
		return 100
	case r >= 3:
		return 80
	case r >= 2:
		return 60
	default:
		return 40
	}
}

// escalatingPower — «нарастающие» атаки:
//   279 Накопленная энергия — 20 + 20 за каждую ступень ПОВЫШЕНИЯ статов польз.
//   355 Эхо-Волны — 40 за каждый ход подряд этой атакой (макс 200).
func escalatingPower(attacker *BattlePokemon, move *BattleMove) (int, bool) {
	switch move.ID {
	case 279:
		boosts := 0
		for i := 1; i <= 7; i++ {
			if attacker.StatStages[i] > 0 {
				boosts += attacker.StatStages[i]
			}
		}
		return 20 + 20*boosts, true
	case 355:
		p := 40 * attacker.SameStreak
		if p < 40 {
			p = 40
		}
		if p > 200 {
			p = 200
		}
		return p, true
	case 293: // Плевок — 100 мощности за каждое очко запаса (Stockpile>0 гарантирован выше)
		return 100 * attacker.Stockpile, true
	case 155: // Ледяной шар (Rollout) — ×2 за каждый ход подряд: 30..480 (макс 5)
		s := attacker.SameStreak
		if s < 1 {
			s = 1
		}
		if s > 5 {
			s = 5
		}
		return 30 * (1 << uint(s-1)), true
	}
	return 0, false
}

// movePower — эффективная мощность хода: нарастающие (Накопленная энергия/
// Эхо-Волны), вес-зависимые (Подножка/Пресс/Путы), HP-зависимые (Водный смерч/
// Извержение), затем базовая с возможным условным удвоением.
func movePower(attacker, defender *BattlePokemon, move *BattleMove) int {
	if p, ok := escalatingPower(attacker, move); ok {
		return p
	}
	if p, ok := weightBasedPower(attacker, defender, move); ok {
		return p
	}
	if base, ok := userHPPowerMoves[move.ID]; ok {
		p := base * attacker.CurrentHP / attacker.MaxHP
		if p < 1 {
			p = 1
		}
		return p
	}
	if base, ok := foeHPPowerMoves[move.ID]; ok { // Захват — по HP цели
		p := base * defender.CurrentHP / defender.MaxHP
		if p < 1 {
			p = 1
		}
		return p
	}
	if move.ID == 262 { // Козырь — мощность от остатка PP
		return trumpCardPower(move)
	}
	if move.ID == 351 { // Электрический шар — по отношению скоростей
		return electroBallPower(attacker, defender)
	}
	p := move.Power
	if powerDoubled(defender, move) {
		p *= 2
	}
	return p
}

// electroBallPower — Электрический шар(351): чем пользователь быстрее цели, тем
// сильнее (60..150). Считаем по ЭФФЕКТИВНОЙ Скорости (база * ступень).
func electroBallPower(attacker, defender *BattlePokemon) int {
	foe := effectiveStat(defender.Speed, defender.StatStages[3])  // 3 = Скорость
	self := effectiveStat(attacker.Speed, attacker.StatStages[3]) // 3 = Скорость
	if self < 1 {
		self = 1
	}
	if foe < 1 {
		foe = 1
	}
	switch r := float64(foe) / float64(self); {
	case r <= 0.25: // пользователь ≥4× быстрее
		return 150
	case r <= 1.0/3.0:
		return 120
	case r <= 0.5:
		return 100
	case r <= 1.0: // пользователь быстрее или равен
		return 80
	default: // пользователь медленнее
		return 60
	}
}

// trumpCardPower — Козырь(262): чем меньше PP осталось, тем сильнее (40..190).
// PP берётся ДО списания за этот ход (списывается в конце хода), поэтому
// последний заряд = PP 1 -> максимум. У дикого PP бесконечен (<0) -> базовые 40.
func trumpCardPower(move *BattleMove) int {
	pp := move.PP
	if pp < 0 {
		return 40
	}
	switch {
	case pp <= 1:
		return 190
	case pp == 2:
		return 80
	case pp == 3:
		return 60
	case pp == 4:
		return 50
	default:
		return 40
	}
}

// freezeDryEffectiveness — эффективность Сухого льда(329): по Воде(3) всегда ×2,
// по остальным типам — обычная эффективность Льда(6). Перемножается по типам цели.
func freezeDryEffectiveness(defTypes []int) float64 {
	eff := 1.0
	for _, dt := range defTypes {
		if dt == 3 { // Вода
			eff *= 2.0
		} else {
			eff *= getTypeEffectiveness(6, []int{dt}) // Лёд
		}
	}
	return eff
}

// applyClearSmog — Грязевая ванна(240): при попадании сбрасывает ВСЕ изменения
// боевых характеристик цели в ноль.
func applyClearSmog(defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if move.ID != 240 || defender == nil {
		return
	}
	changed := false
	for i := range defender.StatStages {
		if defender.StatStages[i] != 0 {
			defender.StatStages[i] = 0
			changed = true
		}
	}
	if changed {
		*log = append(*log, BattleLogEntry{
			Round:      round,
			StatChange: "Все изменения статов " + defender.Name + " сброшены!",
		})
	}
}
