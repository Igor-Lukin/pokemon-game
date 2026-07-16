package api

// type_moves.go — смена ТИПА пользователя в бою (перекрытие через BattleTypes,
// которое учитывает getPokemonTypes; сбрасывается при смене покемона/конце боя):
//   454 Адаптация (Conversion)    — тип = тип одной из изученных атак (первой).
//   468 Дублирование (Reflect Type) — тип = тип(ы) противника.
//   455 Адаптация-2 (Conversion 2) — тип, устойчивый к последней атаке ПО себе.
// Смена типа ЦЕЛИ (BattleTypes на defender; сброс при смене цели):
//   465 Гнев леса (Forest's Curse) — добавить цели Травяной тип.
//   533 Сласти-или-напасти (Trick-or-Treat) — добавить цели Призрачный тип.
//   491 Ливень (Soak) — сменить тип цели на Водный.
// (Камуфляж 481 — отдельно, зависит от локации.)

const (
	moveConversion  = 454
	moveReflectType = 468
	moveConversion2 = 455
	moveForestsCurse = 465 // Гнев леса — добавить цели Травяной тип
	moveTrickOrTreat = 533 // Сласти-или-напасти — добавить цели Призрачный тип
	moveSoak         = 491 // Ливень — сменить тип цели на Водный
)

var typeNames = map[int]string{
	1: "Нормальный", 2: "Огненный", 3: "Водный", 4: "Электрический", 5: "Травяной",
	6: "Ледяной", 7: "Боевой", 8: "Ядовитый", 9: "Земляной", 10: "Летающий",
	11: "Психический", 12: "Жучиный", 13: "Каменный", 14: "Призрачный", 15: "Драконий",
	16: "Тёмный", 17: "Стальной", 18: "Волшебный",
}

func typeName(id int) string {
	if n, ok := typeNames[id]; ok {
		return n
	}
	return "неизвестный"
}

// handleTypeMove — статусные ходы смены типа. Возвращает true, если обработан.
func handleTypeMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case moveConversion:
		conversionMove(attacker, round, log)
	case moveReflectType:
		reflectTypeMove(attacker, defender, round, log)
	case moveConversion2:
		conversion2Move(attacker, round, log)
	case moveForestsCurse:
		addTypeToTarget(defender, 5, "Травяной", round, log)
	case moveTrickOrTreat:
		addTypeToTarget(defender, 14, "Призрачный", round, log)
	case moveSoak:
		if defender == nil {
			return true
		}
		setBattleTypes(defender, []int{3}, round, log) // тип цели -> только Водный
	default:
		return false
	}
	return true
}

// addTypeToTarget — добавить цели ещё один тип (Гнев леса/Сласти-или-напасти),
// сохранив прежние. Повторное добавление того же типа — без эффекта.
func addTypeToTarget(t *BattlePokemon, addType int, addName string, round int, log *[]BattleLogEntry) {
	if t == nil {
		return
	}
	cur := getPokemonTypes(t)
	for _, ty := range cur {
		if ty == addType {
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: t.Name, MoveFailed: "У " + t.Name + " уже есть этот тип — не сработало!",
			})
			return
		}
	}
	nt := make([]int, len(cur)+1)
	copy(nt, cur)
	nt[len(cur)] = addType
	t.BattleTypes = nt
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: t.Name, PokemonImage: t.ImageURL,
		Message: "К " + t.Name + " добавлен " + addName + " тип!",
	})
}

// setBattleTypes — установить боевой тип и залогировать.
func setBattleTypes(p *BattlePokemon, types []int, round int, log *[]BattleLogEntry) {
	p.BattleTypes = types
	names := ""
	for i, t := range types {
		if i > 0 {
			names += "/"
		}
		names += typeName(t)
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " сменил тип на " + names + "!",
	})
}

// conversionMove — Адаптация(454): тип = тип первой изученной атаки (кроме самой
// Адаптации, если есть другие ходы).
func conversionMove(attacker *BattlePokemon, round int, log *[]BattleLogEntry) {
	newType := 0
	for _, m := range attacker.Moves {
		if m.ID == moveConversion {
			continue
		}
		newType = m.TypeID
		break
	}
	if newType == 0 && len(attacker.Moves) > 0 {
		newType = attacker.Moves[0].TypeID
	}
	if newType == 0 {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, Message: "Адаптация не удалась."})
		return
	}
	setBattleTypes(attacker, []int{newType}, round, log)
}

// reflectTypeMove — Дублирование(468): тип = тип(ы) противника.
func reflectTypeMove(attacker, defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	if defender == nil {
		return
	}
	src := getPokemonTypes(defender)
	cp := make([]int, len(src))
	copy(cp, src)
	setBattleTypes(attacker, cp, round, log)
}

// conversion2Move — Адаптация-2(455): тип, устойчивый (эффективность <1) к типу
// последней атаки ПО пользователю.
func conversion2Move(attacker *BattlePokemon, round int, log *[]BattleLogEntry) {
	hitType := attacker.LastHitByType
	if hitType == 0 {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, Message: "Адаптации-2 не к чему приспособиться."})
		return
	}
	for t := 1; t <= 18; t++ {
		if getTypeEffectiveness(hitType, []int{t}) < 1.0 {
			setBattleTypes(attacker, []int{t}, round, log)
			return
		}
	}
	*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, Message: "Адаптация-2 не нашла устойчивого типа."})
}
