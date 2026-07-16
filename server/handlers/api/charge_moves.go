package api

import "fmt"

// charge_moves.go — двухходовые атаки «зарядка → удар». Первый ход покемон
// готовится (Солнечный луч копит энергию; Полёт/Погружение/Подкоп/Полтергейст/
// Прыжок уходят в неуязвимую фазу), урона нет. На следующем ходу покемон
// АВТОМАТИЧЕСКИ бьёт сохранённой атакой (выбор игрока в этот ход игнорируется);
// состояние сбрасывается.
//
// Состояние на BattlePokemon: ChargingMoveID (какой ход заряжается) +
// Invulnerable (в неуязвимой фазе — все атаки оппонента мимо, пока не выйдет).
//
// Исключения-«пробиватели» неуязвимости (Воздушный апперкот по летящим,
// Землетрясение по зарытым и т.п.) пока НЕ реализованы — в фазе неуязвимости
// мимо проходят ВСЕ атаки оппонента. Добавим отдельно.

type chargeDef struct {
	invuln      bool   // уходит ли в неуязвимую фазу в ход зарядки
	sunSkip     bool   // пропускать зарядку в солнечную погоду (Солнечный луч)
	chargeMsg   string // сообщение лога в ход зарядки (%s = имя покемона)
	chargeStat  int    // если >0 — на ходу зарядки поднять этот стат (id как в StatStages)
	chargeStage int    // на сколько ступеней поднять chargeStat
}

// chargeMoves — реестр двухходовых атак по id. Набор фиксированный.
var chargeMoves = map[int]chargeDef{
	20:  {invuln: false, sunSkip: true, chargeMsg: "%s накапливает солнечную энергию!"}, // Солнечный луч
	202: {invuln: true, chargeMsg: "%s взмыл высоко в воздух!"},                          // Полет
	203: {invuln: true, chargeMsg: "%s нырнул глубоко под воду!"},                        // Погружение
	206: {invuln: true, chargeMsg: "%s зарылся под землю!"},                              // Подкоп
	211: {invuln: true, chargeMsg: "%s растворился в потустороннем мире!"},               // Полтергейст
	219: {invuln: true, chargeMsg: "%s высоко подпрыгнул вверх!"},                        // Прыжок
	25:  {invuln: false, chargeMsg: "%s становится в боевую стойку, готовясь к удару!", chargeStat: 2, chargeStage: 1}, // Торпеда: Защита +1
	176: {invuln: true, chargeMsg: "%s хватает противника и взмывает в небо!"},          // Небесный бросок (Sky Drop)
}

const moveSkyDrop = 176

// skyDropGrab — Небесный бросок, ход захвата: цель поднята в небо (неуязвима и
// пропускает следующий ход). Пользователь уходит в неуязвимость обычной charge-логикой.
func skyDropGrab(defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	defender.Invulnerable = true
	defender.SkyHeld = true
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
		Message: defender.Name + " схвачен и поднят в небо!",
	})
}

// skyDropRelease — отпустить цель Небесного броска (после удара или срыва).
func skyDropRelease(defender *BattlePokemon) {
	defender.Invulnerable = false
	defender.SkyHeld = false
}

// releaseStaleSkyHold — страховка: если удерживаемый ещё висит, а держащий уже
// НЕ заряжает Небесный бросок (срыв/смена), снять удержание.
func releaseStaleSkyHold(a, b *BattlePokemon) {
	if a.SkyHeld && b.ChargingMoveID != moveSkyDrop {
		skyDropRelease(a)
	}
	if b.SkyHeld && a.ChargingMoveID != moveSkyDrop {
		skyDropRelease(b)
	}
}

// rechargeMoves — атаки, после применения которых покемон пропускает следующий
// ход (перезарядка): Гиперлуч и т.п. Пропуск ставится даже при промахе. Сам
// пропуск обрабатывается в processPreTurnStatus по флагу MustRecharge.
var rechargeMoves = map[int]bool{
	70:  true, // Гиперлуч
	228: true, // Взрывная волна
	237: true, // Гидропушка
	242: true, // Гул времён
	243: true, // Дьявольские силки
}

// piercesInvuln — может ли атака moveID попасть по цели в неуязвимой фазе,
// вызванной ходом chargeMoveID (в воздухе/под землёй/…):
//   Землетрясение(74)       -> Подкоп(206, под землёй);
//   Воздушный апперкот(105)  -> Полёт(202)/Прыжок(219, в воздухе).
func piercesInvuln(moveID, chargeMoveID int) bool {
	switch moveID {
	case 74:
		return chargeMoveID == 206
	case 105:
		return chargeMoveID == 202 || chargeMoveID == 219
	}
	return false
}

// findMoveByID — найти ход покемона по id (для принудительного завершения
// двухходовки на ходу-ударе).
func findMoveByID(p *BattlePokemon, id int) *BattleMove {
	for i := range p.Moves {
		if p.Moves[i].ID == id {
			return &p.Moves[i]
		}
	}
	return nil
}

// beginChargeIfNeeded — вызывается перед нанесением урона.
// Возвращает true, если действие израсходовано на ЗАРЯДКУ (урон в этот ход НЕ
// наносится). Возвращает false, если бить нужно прямо сейчас: обычная атака,
// либо ход-удар двухходовки, либо пропуск зарядки Солнечного луча в солнце.
func beginChargeIfNeeded(attacker *BattlePokemon, move *BattleMove, weatherID, round int, log *[]BattleLogEntry) bool {
	// Уже заряжались этой атакой -> это ход-удар: выйти из неуязвимости и бить.
	if attacker.ChargingMoveID == move.ID {
		attacker.ChargingMoveID = 0
		attacker.Invulnerable = false
		return false
	}
	def, ok := chargeMoves[move.ID]
	if !ok {
		return false // обычная одноходовая атака
	}
	// Солнечный луч в солнце — бьём сразу, без зарядки.
	if def.sunSkip && weatherID == weatherSun {
		return false
	}
	// Ход зарядки: запомнить атаку, уйти в неуязвимость (если положено), лог.
	attacker.ChargingMoveID = move.ID
	attacker.Invulnerable = def.invuln
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  attacker.Name,
		PokemonImage: attacker.ImageURL,
		Message:      fmt.Sprintf(def.chargeMsg, attacker.Name),
	})
	// Буст стата на ходу зарядки (напр. Торпеда: Защита +1). curseBump —
	// общий помощник ступени стата с логом (клэмп [-6,6], без "резко" для ±1).
	if def.chargeStat > 0 {
		curseBump(attacker, def.chargeStat, def.chargeStage, round, log)
	}
	return true
}

// chooseWildMoveOrCharge — если дикий покемон заряжается, он обязан завершить
// свою атаку; иначе выбирает ход обычным образом.
func chooseWildMoveOrCharge(wild *BattlePokemon) *BattleMove {
	if wild.ChargingMoveID != 0 {
		if m := findMoveByID(wild, wild.ChargingMoveID); m != nil {
			return m
		}
	}
	// Захват мультиход-локом (Ледяной шар/Танец/Шум) — обязан повторить.
	if wild.LockedMoveID != 0 {
		if m := findMoveByID(wild, wild.LockedMoveID); m != nil {
			return m
		}
	}
	// Выжидание: пока идёт ожидание — форсированно продолжает (counter_moves.go).
	if wild.BideTurns > 0 {
		if m := findMoveByID(wild, moveBide); m != nil {
			return m
		}
	}
	// Овации/Приказ: форсированно повторяет EncoreMoveID (block_moves.go).
	if wild.EncoreTurns != 0 && wild.EncoreMoveID != 0 {
		if m := findMoveByID(wild, wild.EncoreMoveID); m != nil {
			return m
		}
	}
	return chooseWildMove(wild)
}
