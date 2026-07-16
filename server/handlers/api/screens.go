package api

// screens.go — «сторона поля»: эффекты, которые живут не на покемоне, а на
// СТОРОНЕ боя (у игрока и у дикого — свои независимые таймеры). Слои:
//   - экраны урона: Защитный экран (физ.) и Экран света (спец.) — входящий
//     урон своей категории ×0.5 на 5 раундов, крит пробивает;
//   - Попутный ветер: Скорость стороны ×2 на 4 раунда (влияет на порядок хода).
// Это фундамент под будущую «полевую стопку» (Ограждение, арены и т.п.) —
// сюда же будут добавляться новые таймеры.
//
// Все ходы-на-сторону — статусные (кат.3, power 0): урона не наносят, ставят
// таймер на сторону пользователя. Множитель урона применяется в calculateDamage
// к обычной формуле; фикс./OHKO-урон идёт в обход формулы и экранами не режется.

const (
	moveReflect      = 360 // Защитный экран — физ. урон в сторону ×0.5
	moveLightScreen  = 369 // Экран света    — спец. урон в сторону ×0.5
	moveTailwind     = 441 // Попутный ветер — Скорость стороны ×2
	moveSafeguard    = 362 // Ограждение     — защита стороны от статусов
	moveMist         = 366 // Туман          — защита стороны от снижения статов
	moveDefog        = 420 // Чистое небо    — снос всего с ОБЕИХ сторон (статусная)
	moveBrickBreak   = 76  // Прорыв барьера — урон + слом экранов цели
	moveRapidSpin    = 87  // Вращение       — урон + снос всего с обеих сторон
	screenDuration   = 5   // раундов действия экрана
	tailwindDuration = 4   // раундов действия Попутного ветра
	guardDuration    = 5   // раундов действия Ограждения / Тумана
)

// SideState — состояние одной стороны поля. Таймеры у игрока и дикого свои,
// поэтому переживают смену покемона игрока (эффект привязан к стороне, не к
// покемону). *SetTurn — раунд установки: в него таймер не убавляем, иначе
// длительность сразу уменьшится на 1 (тот же приём, что у погоды с WeatherSetTurn).
type SideState struct {
	ReflectTurns       int // Защитный экран: физ. урон в эту сторону ×0.5
	ReflectSetTurn     int
	LightScreenTurns   int // Экран света: спец. урон в эту сторону ×0.5
	LightScreenSetTurn int
	TailwindTurns      int // Попутный ветер: Скорость этой стороны ×2
	TailwindSetTurn    int
	SafeguardTurns     int // Ограждение: сторона защищена от статусов противника
	SafeguardSetTurn   int
	MistTurns          int // Туман: сторона защищена от снижения статов противником
	MistSetTurn        int
	// Ловушки на поле (hazards.go) — бьют покемона, ВХОДЯЩЕГО на эту сторону.
	// Не имеют таймера: держатся весь бой (снос — отдельным шагом). Ставятся на
	// сторону ПРОТИВНИКА.
	SpikesLayers      int  // Шипы: слои 0..3 (урон наземным)
	ToxicSpikesLayers int  // Отравленные шипы: слои 0..2 (яд/сильный яд наземным)
	StealthRock       bool // Каменная ловушка: урон по эффективности Камня
	StickyWeb         bool // Клейкая паутина: Скорость -1 наземному входящему
	// Исцеляющее желание/Лунный танец: пользователь пожертвовал собой — СЛЕДУЮЩИЙ
	// вошедший на эту сторону покемон будет полностью исцелён (heal_moves.go).
	HealingWishPending bool
	LuckyChantTurns    int // Наговор: по этой стороне нельзя крит (field_effects.go)
	LuckyChantSetTurn  int
}

// sideFor — сторона указанного покемона: активный покемон игрока -> PlayerSide,
// любой другой (дикий) -> WildSide.
func sideFor(bs *BattleState, p *BattlePokemon) *SideState {
	if p == &bs.PlayerTeam[bs.ActiveIdx] {
		return &bs.PlayerSide
	}
	return &bs.WildSide
}

// applySideMove — если ход ставит эффект на сторону поля, заводит его и пишет в
// лог. Возвращает true, если ход обработан. Эффекты СВОЕЙ стороны (экраны,
// Попутный ветер, Ограждение, Туман) не стакаются — повторное применение
// обновляет длительность. Ловушки (Шипы/Отравленные шипы/Каменная ловушка/
// Клейкая паутина) ставятся на сторону ПРОТИВНИКА и обрабатываются отдельно
// (applyHazardMove) — они стакаются слоями и бьют входящего покемона.
func applySideMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, log *[]BattleLogEntry) bool {
	if applyHazardMove(bs, attacker, defender, move, log) {
		return true
	}
	// Чистое небо: сносит экраны/защиту/ловушки с ОБЕИХ сторон (статусная атака).
	if move.ID == moveDefog {
		clearFieldBothSides(bs, bs.Turn, log)
		return true
	}
	// Огнеупорная пыльца: волатиль на ЦЕЛИ (fire_effects.go), не на сторону.
	if move.ID == movePowder {
		applyPowderMove(defender, bs.Turn, log)
		return true
	}
	// Увлажнение: общеполевое ослабление Огня (fire_effects.go), не на сторону.
	if move.ID == moveWaterSport {
		bs.WaterSportTurns = waterSportDuration
		bs.WaterSportSetTurn = bs.Turn
		*log = append(*log, BattleLogEntry{
			Round:        bs.Turn,
			PokemonName:  attacker.Name,
			PokemonImage: attacker.ImageURL,
			Message:      "Поле пропиталось влагой — огненные атаки ослаблены!",
		})
		return true
	}
	// Общеполевые/самоличные таймеры (field_effects.go).
	switch move.ID {
	case moveMudSport:
		bs.MudSportTurns = fieldEffectDuration
		bs.MudSportSetTurn = bs.Turn
		fieldLog(attacker, "Поле осушено электростатикой — электрические атаки ослаблены!", bs.Turn, log)
		return true
	case moveGravity:
		bs.GravityTurns = fieldEffectDuration
		bs.GravitySetTurn = bs.Turn
		fieldLog(attacker, "Гравитация усилилась — Летающие прижаты к земле!", bs.Turn, log)
		return true
	case moveMagnetRise:
		attacker.MagnetRiseTurns = fieldEffectDuration
		fieldLog(attacker, attacker.Name+" поднялся на магнитном поле — неуязвим для Земли!", bs.Turn, log)
		return true
	case moveLuckyChant:
		s := sideFor(bs, attacker)
		s.LuckyChantTurns = fieldEffectDuration
		s.LuckyChantSetTurn = bs.Turn
		fieldLog(attacker, "Прозвучал наговор — крит по стороне невозможен!", bs.Turn, log)
		return true
	}
	// Арены/терраины (terrain_moves.go) — общеполевой таймер, взаимоисключающие.
	if terrain, ok := terrainByMove[move.ID]; ok {
		setTerrain(bs, terrain, bs.Turn, log)
		return true
	}
	// Комнаты/поле/Плантаж (room_moves.go).
	if handleRoomMove(bs, attacker, defender, move, log) {
		return true
	}
	side := sideFor(bs, attacker)
	var msg string
	switch move.ID {
	case moveReflect:
		side.ReflectTurns = screenDuration
		side.ReflectSetTurn = bs.Turn
		msg = "Защитный экран окутал сторону — физический урон ослаблен!"
	case moveLightScreen:
		side.LightScreenTurns = screenDuration
		side.LightScreenSetTurn = bs.Turn
		msg = "Поднялся Экран света — специальный урон ослаблен!"
	case moveTailwind:
		side.TailwindTurns = tailwindDuration
		side.TailwindSetTurn = bs.Turn
		msg = "Подул попутный ветер — Скорость стороны удвоена!"
	case moveSafeguard:
		side.SafeguardTurns = guardDuration
		side.SafeguardSetTurn = bs.Turn
		msg = "Ограждение укрыло сторону — статусы противника не пройдут!"
	case moveMist:
		side.MistTurns = guardDuration
		side.MistSetTurn = bs.Turn
		msg = "Сторону окутал туман — статы не снизить!"
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

// screenDamageMultiplier — множитель входящего урона по экрану защищающейся
// стороны: ×0.5 для физической атаки под Защитным экраном и для специальной
// под Экраном света. Крит пробивает экран (множитель 1). Без экрана — 1.0.
func screenDamageMultiplier(defSide *SideState, move *BattleMove, isCrit bool) float64 {
	if defSide == nil || isCrit {
		return 1.0
	}
	if move.CategoryID == 1 && defSide.ReflectTurns > 0 { // физическая
		return 0.5
	}
	if move.CategoryID == 2 && defSide.LightScreenTurns > 0 { // специальная
		return 0.5
	}
	return 1.0
}

// safeguardBlocks — блокирует ли Ограждение наложение эффекта с данным EffectID:
// основные статусы (2-7: паралич/ожог/заморозка/сон/яд/сильный яд) и
// замешательство (13). Пиявки/ловушка/испуг (8/10/11) проходят.
func safeguardBlocks(effectID int) bool {
	return (effectID >= 2 && effectID <= 7) || effectID == 13
}

// tailwindSpeed — эффективная Скорость покемона с учётом Попутного ветра на его
// стороне (×2). Используется при определении порядка хода. Оборачивает
// effectiveSpeed (статус/бусты Скорости считаются там).
func tailwindSpeed(bs *BattleState, p *BattlePokemon) int {
	s := effectiveSpeed(p)
	if sideFor(bs, p).TailwindTurns > 0 {
		s *= 2
	}
	return s
}

// tickSideTimers — конец хода: убавляем таймеры стороны у обеих сторон и пишем в
// лог момент спадания. Вызывается один раз за ход рядом с postProcessWeather.
func tickSideTimers(bs *BattleState, log *[]BattleLogEntry) {
	tickOneSide(&bs.PlayerSide, "Ваша сторона: ", bs.Turn, log)
	tickOneSide(&bs.WildSide, "Сторона дикого: ", bs.Turn, log)
	// Увлажнение — общеполевой таймер (fire_effects.go); в раунд установки не тикаем.
	if bs.WaterSportTurns > 0 && bs.Turn != bs.WaterSportSetTurn {
		bs.WaterSportTurns--
		if bs.WaterSportTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: bs.Turn, Message: "Влага испарилась — Огонь снова в полную силу."})
		}
	}
	// Огнеупорная пыльца действует только текущий ход — снимаем в конце хода с
	// обоих активных покемонов (fire_effects.go).
	bs.PlayerTeam[bs.ActiveIdx].Powdered = false
	bs.WildPokemon.Powdered = false
	// Насест — Летающий тип теряется только на текущий ход (heal_moves.go).
	bs.PlayerTeam[bs.ActiveIdx].Roosting = false
	bs.WildPokemon.Roosting = false
	// Отложенное лечение Желания (heal_moves.go).
	processWishHeals(bs, log)
	// Снятие защитных щитов + перенос «удачной защиты» на приоритет след. хода
	// (protect_moves.go).
	endTurnProtect(&bs.PlayerTeam[bs.ActiveIdx])
	endTurnProtect(bs.WildPokemon)
	// Таймеры Овации/Запрета (block_moves.go).
	tickBlockTimers(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickBlockTimers(bs.WildPokemon, bs.Turn, log)
	// Запрет регенерации — обратный отсчёт на обоих активных (heal_moves.go).
	tickHealBlock(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickHealBlock(bs.WildPokemon, bs.Turn, log)
	// Скорбная песнь / истечение Связи судеб (sacrifice_moves.go).
	tickPerish(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickPerish(bs.WildPokemon, bs.Turn, log)
	tickDestinyBond(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickDestinyBond(bs.WildPokemon, bs.Turn, log)
	tickGrudge(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickGrudge(bs.WildPokemon, bs.Turn, log)
	// Общеполевые таймеры Осушения/Гравитации + Магнетизм (field_effects.go).
	tickGlobalFieldEffects(bs, log)
	tickMagnetRise(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickMagnetRise(bs.WildPokemon, bs.Turn, log)
	tickLevitate(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickLevitate(bs.WildPokemon, bs.Turn, log)
	// Зевок — отложенный сон (status_extra_moves.go).
	tickYawn(&bs.PlayerTeam[bs.ActiveIdx], bs.Turn, log)
	tickYawn(bs.WildPokemon, bs.Turn, log)
	// Электризация действует только текущий ход — снимаем непотраченный флаг.
	bs.PlayerTeam[bs.ActiveIdx].Electrified = false
	bs.WildPokemon.Electrified = false
	// Арена/терраин: лечение Травяной + отсчёт (terrain_moves.go).
	tickTerrain(bs, log)
	// Комнаты/поле — отсчёт (room_moves.go).
	tickRooms(bs, log)
}

func tickOneSide(s *SideState, who string, round int, log *[]BattleLogEntry) {
	if s.ReflectTurns > 0 && round != s.ReflectSetTurn {
		s.ReflectTurns--
		if s.ReflectTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: round, Message: who + "Защитный экран рассеялся."})
		}
	}
	if s.LightScreenTurns > 0 && round != s.LightScreenSetTurn {
		s.LightScreenTurns--
		if s.LightScreenTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: round, Message: who + "Экран света погас."})
		}
	}
	if s.TailwindTurns > 0 && round != s.TailwindSetTurn {
		s.TailwindTurns--
		if s.TailwindTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: round, Message: who + "Попутный ветер стих."})
		}
	}
	if s.SafeguardTurns > 0 && round != s.SafeguardSetTurn {
		s.SafeguardTurns--
		if s.SafeguardTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: round, Message: who + "Ограждение рассеялось."})
		}
	}
	if s.MistTurns > 0 && round != s.MistSetTurn {
		s.MistTurns--
		if s.MistTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: round, Message: who + "Туман рассеялся."})
		}
	}
	if s.LuckyChantTurns > 0 && round != s.LuckyChantSetTurn {
		s.LuckyChantTurns--
		if s.LuckyChantTurns == 0 {
			*log = append(*log, BattleLogEntry{Round: round, Message: who + "Наговор развеялся."})
		}
	}
}

// ---------------------------------------------------------------------------
// Снос полевых эффектов: Чистое небо(420)/Вращение(87) — всё с обеих сторон;
// Прорыв барьера(76) — только экраны цели. Попутный ветер НЕ сносится (это не
// барьер и в описании сноса его нет).
// ---------------------------------------------------------------------------

// clearOneSide — снять со стороны экраны, Ограждение/Туман и все ловушки.
// Возвращает число снятых эффектов (для решения, писать ли в лог).
func clearOneSide(s *SideState) int {
	n := 0
	if s.ReflectTurns > 0 {
		s.ReflectTurns = 0
		n++
	}
	if s.LightScreenTurns > 0 {
		s.LightScreenTurns = 0
		n++
	}
	if s.SafeguardTurns > 0 {
		s.SafeguardTurns = 0
		n++
	}
	if s.MistTurns > 0 {
		s.MistTurns = 0
		n++
	}
	if s.SpikesLayers > 0 {
		s.SpikesLayers = 0
		n++
	}
	if s.ToxicSpikesLayers > 0 {
		s.ToxicSpikesLayers = 0
		n++
	}
	if s.StealthRock {
		s.StealthRock = false
		n++
	}
	if s.StickyWeb {
		s.StickyWeb = false
		n++
	}
	return n
}

// clearBothSides — снос полевых эффектов с двух сторон (ядро Чистого неба и
// Вращения). В 1v1 две стороны — атакующего и защищающегося.
func clearBothSides(a, b *SideState, round int, log *[]BattleLogEntry) {
	n := clearOneSide(a) + clearOneSide(b)
	if n > 0 {
		*log = append(*log, BattleLogEntry{
			Round:   round,
			Message: "Порыв расчистил поле — экраны и ловушки сметены с обеих сторон!",
		})
	}
}

// clearFieldBothSides — обёртка для статусного Чистого неба (обе стороны боя).
func clearFieldBothSides(bs *BattleState, round int, log *[]BattleLogEntry) {
	clearBothSides(&bs.PlayerSide, &bs.WildSide, round, log)
}

// breakScreensOn — сломать ТОЛЬКО экраны (Защитный экран + Экран света) на
// стороне. Для Прорыва барьера: вызывается ДО расчёта урона, чтобы свой удар
// не был ослаблен ломаемым экраном. Ловушки/Ограждение/Туман не трогает.
func breakScreensOn(side *SideState, round int, log *[]BattleLogEntry) {
	if side == nil {
		return
	}
	n := 0
	if side.ReflectTurns > 0 {
		side.ReflectTurns = 0
		n++
	}
	if side.LightScreenTurns > 0 {
		side.LightScreenTurns = 0
		n++
	}
	if n > 0 {
		*log = append(*log, BattleLogEntry{Round: round, Message: "Защитные барьеры цели разбиты!"})
	}
}
