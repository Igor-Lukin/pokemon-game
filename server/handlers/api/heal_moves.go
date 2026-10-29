package api

import "strconv"

// heal_moves.go — оставшиеся лечащие механики (кроме простых %-самолечений,
// которые сделаны данными heal_percent):
//   82  Отдых (Rest)          — полностью лечит + сон на 2 хода + снятие статусов.
//   368 Укоренение (Ingrain)  — +1/16 макс. HP в конце хода; нельзя сменить покемона.
//   459 Водный покров (Aqua Ring) — +1/16 макс. HP в конце хода.
//   470 Желание (Wish)        — +1/2 макс. HP отложенно, в следующем ходу.
//   489 Крик смерти (Pain Split) — суммирует HP обоих и делит поровну.
//   507 Очищение (Refresh)    — снять свой паралич/ожог/яд.
//   467 Детокс (Purify)       — снять статус противника + вылечить себя 25%.
//   530 Силоотток (Strength Sap) — вылечить себя на Атаку врага + Атака врага -1.
//   456 Ароматерапия / 479 Исцеляющий звон — снять статусы у ВСЕЙ своей команды.
//   478 Исцеляющее желание / 494 Лунный танец — пользователь падает в обморок,
//       ВХОДЯЩИЙ на его сторону покемон полностью исцеляется (флаг на SideState).
//   472 Запрет регенерации (Heal Block) — цель не может лечить HP 5 ходов
//       (isHealBlocked гасит applyHeal/healBy/restMove/over-time/Wish; tickHealBlock).
//   72  Насест (Roost)        — лечит 50% + до конца хода снимает Летающий тип
//       (флаг Roosting; getPokemonTypes фильтрует тип10, гасится в tickSideTimers).
// (простые %-самолечения — данными heal_percent: 526 Регенерация=50, 544 Импульс=33.)
// Диспетчер — handleHealMove (вызывается из handleCustomStatusMove). Отложенное
// Желание и по-ходовое лечение Укоренения/Покрова обрабатываются в конце хода.

const (
	moveRest          = 82
	moveIngrain       = 368
	moveAquaRing      = 459
	moveWish          = 470
	movePainSplit     = 489
	moveRefresh       = 507 // снять свой паралич/ожог/яд
	movePurify        = 467 // снять статус противника + вылечить себя 25%
	moveStrengthSap   = 530 // вылечить себя на Атаку противника + Атака противника -1
	moveAromatherapy  = 456 // снять статусы у всей своей команды
	moveHealBell      = 479 // снять статусы у всей своей команды
	moveHealingWish   = 478 // жертва собой: входящий покемон полностью исцеляется
	moveLunarDance    = 494 // то же (жертвенное возрождение входящего)
	moveHealBlock     = 472 // Запрет регенерации: цель не лечится N ходов
	moveRoost         = 72  // Насест: лечит 50% + до конца хода теряет Летающий тип
	healBlockDuration = 5   // раундов действия Запрета регенерации
	overTimeHealDenom = 16  // 1/16 макс. HP за ход (Укоренение/Водный покров)
	restSleepTurns    = 2   // Отдых спит фиксированно 2 хода
)

// handleHealMove — кастомные лечащие статусные ходы. defender нужен только Крику
// смерти (усреднение HP). Возвращает true, если ход обработан здесь.
func handleHealMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case moveRest:
		restMove(attacker, round, log)
	case moveIngrain:
		setHealVolatile(attacker, &attacker.Rooted, "пускает корни в поле — будет лечиться каждый ход!", round, log)
	case moveAquaRing:
		setHealVolatile(attacker, &attacker.AquaRing, "окутывается водным покровом — будет лечиться каждый ход!", round, log)
	case moveWish:
		wishMove(attacker, round, log)
	case movePainSplit:
		painSplit(attacker, defender, round, log)
	case moveRefresh:
		refreshMove(attacker, round, log)
	case movePurify:
		purifyMove(attacker, defender, round, log)
	case moveStrengthSap:
		strengthSap(attacker, defender, round, log)
	case moveAromatherapy, moveHealBell:
		teamStatusCure(bs, attacker, round, log)
	case moveHealingWish, moveLunarDance:
		healingWishMove(bs, attacker, round, log)
	case moveHealBlock:
		healBlockMove(defender, round, log)
	case moveRoost:
		roostMove(attacker, round, log)
	default:
		return false
	}
	return true
}

// roostMove — Насест: лечит 50% макс. HP и до конца хода снимает Летающий тип
// (флаг Roosting гасится в tickSideTimers). Под Запретом регенерации лечения нет,
// но тип всё равно теряется на ход.
func roostMove(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	healBy(p, p.MaxHP/2, p.Name+" устраивается на насест и восстанавливает HP!", round, log)
	p.Roosting = true
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " до конца хода теряет Летающий тип.",
	})
}

// teamStatusCure — Ароматерапия/Исцеляющий звон: снять мажорный статус у всей
// команды пользователя. У дикого команды нет — лечится только он сам.
func teamStatusCure(bs *BattleState, user *BattlePokemon, round int, log *[]BattleLogEntry) {
	cured := 0
	clear := func(p *BattlePokemon) {
		if p != nil && p.StatusID >= 2 && p.StatusID <= 7 {
			p.StatusID = 1
			cured++
		}
	}
	if user == bs.WildPokemon {
		clear(user)
	} else {
		for i := range bs.PlayerTeam {
			clear(&bs.PlayerTeam[i])
		}
	}
	if cured > 0 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: user.Name, PokemonImage: user.ImageURL,
			Message: "Приятный звон снимает недуги команды!",
		})
	} else {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: user.Name, Message: "Но снимать нечего — не сработало!",
		})
	}
}

// healingWishMove — Исцеляющее желание/Лунный танец: пользователь падает в обморок,
// а на его сторону вешается флаг — СЛЕДУЮЩИЙ вошедший покемон полностью исцелится
// (HP + снятие статуса). Само исцеление — applyHealingWish при входе.
func healingWishMove(bs *BattleState, user *BattlePokemon, round int, log *[]BattleLogEntry) {
	side := sideFor(bs, user)
	side.HealingWishPending = true
	user.CurrentHP = 0
	user.IsFainted = true
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: user.Name, PokemonImage: user.ImageURL,
		Message: user.Name + " жертвует собой ради исцеления товарища!",
	})
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: user.Name, Fainted: true,
	})
}

// applyHealingWish — при входе покемона на сторону: если там ждёт Исцеляющее
// желание, полностью восстановить HP и снять статус, снять флаг.
func applyHealingWish(side *SideState, p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if side == nil || !side.HealingWishPending || p == nil || p.IsFainted {
		return
	}
	side.HealingWishPending = false
	p.CurrentHP = p.MaxHP
	if p.StatusID >= 2 && p.StatusID <= 7 {
		p.StatusID = 1
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: "Исцеляющее желание сбывается — " + p.Name + " полностью восстановлен!",
	})
}

// healBlockMove — Запрет регенерации: цель не может восстанавливать HP N ходов.
func healBlockMove(foe *BattlePokemon, round int, log *[]BattleLogEntry) {
	if foe == nil {
		return
	}
	if foe.AbilityKey == "aroma_veil" {
		abilityLog(foe, round, log, "", "аромат защищает от Запрета регенерации")
		return
	}
	foe.HealBlockTurns = healBlockDuration
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: foe.Name, PokemonImage: foe.ImageURL,
		Message: foe.Name + " больше не может восстанавливать HP!",
	})
}

// tickHealBlock — обратный отсчёт Запрета регенерации в конце хода; на нуле
// эффект спадает.
func tickHealBlock(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.HealBlockTurns <= 0 {
		return
	}
	p.HealBlockTurns--
	if p.HealBlockTurns == 0 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, Message: p.Name + " снова может восстанавливать HP!",
		})
	}
}

// isHealBlocked — действует ли на покемоне Запрет регенерации (тогда любое
// восстановление HP не проходит). Логирует блокировку.
func isHealBlocked(p *BattlePokemon, round int, log *[]BattleLogEntry) bool {
	if p == nil || p.HealBlockTurns <= 0 {
		return false
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, Message: p.Name + " не может восстановить HP (Запрет регенерации)!",
	})
	return true
}

// healBy — восстановить amount HP носителю (кэп по MaxHP). Если HP уже полны —
// сообщение и без изменений. amount<1 округляется до 1.
func healBy(p *BattlePokemon, amount int, msg string, round int, log *[]BattleLogEntry) {
	if p.IsFainted {
		return
	}
	if isHealBlocked(p, round, log) {
		return
	}
	if p.CurrentHP >= p.MaxHP {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, Message: "HP " + p.Name + " уже полны!",
		})
		return
	}
	if amount < 1 {
		amount = 1
	}
	before := p.CurrentHP
	p.CurrentHP += amount
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Heal: p.CurrentHP - before,
	})
}

// refreshMove — Очищение: снимает СВОЙ паралич(2)/ожог(3)/яд(6)/сильный яд(7).
// Сон(5) и заморозку(4) не снимает (по описанию).
func refreshMove(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	switch p.StatusID {
	case 2, 3, 6, 7:
		p.StatusID = 1
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
			Message: p.Name + " избавляется от своего недуга!",
		})
	default:
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, Message: "Но снимать нечего — не сработало!",
		})
	}
}

// purifyMove — Детокс: если у противника есть мажорный статус (2-7) — снимает его
// и лечит пользователя на 25% макс. HP; иначе провал.
func purifyMove(user, foe *BattlePokemon, round int, log *[]BattleLogEntry) {
	if foe == nil {
		return
	}
	if foe.StatusID >= 2 && foe.StatusID <= 7 {
		foe.StatusID = 1
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: foe.Name, PokemonImage: foe.ImageURL,
			Message: foe.Name + " очищен от недуга!",
		})
		healBy(user, user.MaxHP/4, user.Name+" восстанавливает здоровье!", round, log)
	} else {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: user.Name, Message: "Но у противника нет недуга — не сработало!",
		})
	}
}

// strengthSap — Силоотток: лечит пользователя на величину эффективной Атаки
// противника, затем понижает Атаку противника на -1.
func strengthSap(user, foe *BattlePokemon, round int, log *[]BattleLogEntry) {
	if foe == nil {
		return
	}
	heal := effectiveStat(foe.Attack, foe.StatStages[1])
	healBy(user, heal, user.Name+" вытягивает силы противника и восстанавливает "+strconv.Itoa(heal)+" HP!", round, log)
	curseBump(foe, 1, -1, round, log) // Атака противника -1
}

// restMove — Отдых: полное лечение, сон на 2 хода, снятие мажорного статуса.
func restMove(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if isHealBlocked(p, round, log) {
		return
	}
	if p.CurrentHP >= p.MaxHP {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
			Message: "HP " + p.Name + " уже полны — отдых не нужен!",
		})
		return
	}
	before := p.CurrentHP
	p.CurrentHP = p.MaxHP
	p.StatusID = 5 // сон (сменяет любой прежний мажорный статус — «исцеляет проблемы»)
	p.SleepTurns = restSleepTurns
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " засыпает и полностью восстанавливает HP!",
	})
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Heal: p.CurrentHP - before,
	})
}

// setHealVolatile — включить лечащий волатиль (Укоренение/Водный покров), если
// ещё не активен.
func setHealVolatile(p *BattlePokemon, flag *bool, msg string, round int, log *[]BattleLogEntry) {
	if *flag {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
			Message: "Но эффект уже действует!",
		})
		return
	}
	*flag = true
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " " + msg,
	})
}

// wishMove — Желание: запланировать лечение 1/2 макс. HP на следующий ход.
func wishMove(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p.WishPending {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
			Message: "Но желание уже загадано!",
		})
		return
	}
	p.WishPending = true
	p.WishSetTurn = round
	p.WishHeal = p.MaxHP / 2
	if p.WishHeal < 1 {
		p.WishHeal = 1
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " загадывает желание — исцеление придёт в следующем ходу!",
	})
}

// painSplit — Крик смерти: сумма HP обоих делится поровну (кэп по макс. HP).
func painSplit(user, foe *BattlePokemon, round int, log *[]BattleLogEntry) {
	if foe == nil {
		return
	}
	avg := (user.CurrentHP + foe.CurrentHP) / 2
	setHP := func(p *BattlePokemon) {
		p.CurrentHP = avg
		if p.CurrentHP > p.MaxHP {
			p.CurrentHP = p.MaxHP
		}
		if p.CurrentHP < 0 {
			p.CurrentHP = 0
		}
	}
	setHP(user)
	setHP(foe)
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: user.Name, PokemonImage: user.ImageURL,
		Message: "Боль разделена — HP обоих уравнены до " + strconv.Itoa(avg) + "!",
	})
}

// volatileHealOverTime — конец хода: лечение от Укоренения и Водного покрова
// (по 1/16 макс. HP за каждый активный источник). Регистрируется в endTurnVolatiles.
func volatileHealOverTime(p *BattlePokemon, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted || p.HealBlockTurns > 0 {
		return // Запрет регенерации гасит по-ходовое лечение (молча, без спама)
	}
	if p.Rooted {
		overTimeHeal(p, "корней", log)
	}
	if p.AquaRing {
		overTimeHeal(p, "водного покрова", log)
	}
}

func overTimeHeal(p *BattlePokemon, src string, log *[]BattleLogEntry) {
	if p.CurrentHP >= p.MaxHP {
		return
	}
	heal := p.MaxHP / overTimeHealDenom
	if heal < 1 {
		heal = 1
	}
	p.CurrentHP += heal
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	*log = append(*log, BattleLogEntry{
		Round:   0,
		Message: p.Name + " восстанавливает " + strconv.Itoa(heal) + " HP от " + src + "!",
	})
}

// processWishHeals — конец хода: сработавшие Желания у обоих активных покемонов
// (в раунд загадывания не срабатывает — только со следующего). Вызывается из
// tickSideTimers.
func processWishHeals(bs *BattleState, log *[]BattleLogEntry) {
	wishHealOne(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	wishHealOne(bs.WildPokemon, bs.Turn, log)
}

func wishHealOne(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || !p.WishPending || round == p.WishSetTurn {
		return
	}
	p.WishPending = false
	if p.IsFainted || isHealBlocked(p, round, log) {
		return
	}
	heal := p.WishHeal
	if heal < 1 {
		heal = 1
	}
	if p.CurrentHP >= p.MaxHP {
		return
	}
	p.CurrentHP += heal
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  p.Name,
		PokemonImage: p.ImageURL,
		Message:      "Желание " + p.Name + " сбывается — восстановлено " + strconv.Itoa(heal) + " HP!",
	})
}
