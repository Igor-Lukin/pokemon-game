package api

import "strconv"

// stockpile_moves.go — мини-система Stockpile:
//   500 Накопление — копит заряд (до 3), за каждый +1 Защита и +1 СпЗащита.
//   293 Плевок     — тратит заряд на урон (100 мощности за каждое очко).
//   463 Глоток     — тратит заряд на лечение (1→1/4, 2→1/2, 3→полностью).
// При трате (Плевок/Глоток) заряд обнуляется, а Защита/СпЗащита снижаются на
// столько ступеней, сколько их подняло накопление.
//
// Состояние: BattlePokemon.Stockpile (0..3). Урон Плевка (100*Stockpile)
// считается в movePower; обнуление/фейл — в executeDamagingMove.

// handleCustomStatusMove — кастомные СТАТУСНЫЕ ходы Stockpile (Накопление,
// Глоток). Возвращает true, если ход обработан здесь (у этих ходов нет данных
// эффекта/статов/лечения, поэтому стандартная обработка всё равно пуста).
func handleCustomStatusMove(bs *BattleState, p, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case 500:
		stockpileUp(p, round, log)
		return true
	case 463:
		swallow(p, round, log)
		return true
	}
	// Гендерные атаки (gender_moves.go): Обаяние/Соблазн — гейт по противоположному полу.
	if handleGenderMove(p, defender, move, round, log) {
		return true
	}
	// Защитные ходы (protect_moves.go): Защита/Засечка/Каштановый/Королевский/Терпение.
	if handleProtectMove(p, move, round, log) {
		return true
	}
	// Смена типа (type_moves.go): Адаптация/Дублирование/Адаптация-2.
	if handleTypeMove(p, defender, move, round, log) {
		return true
	}
	// Копирование/обмен ступеней (copy_moves.go): Пародия/Подмена атаки/защиты/
	// Вверх тормашками/Мгла.
	if handleStatCopyMove(p, defender, move, round, log) {
		return true
	}
	// Обмен/усреднение базовых статов (stat_swap_moves.go): Ловкая подмена/
	// Подмена скорости/Разделение атаки/защиты.
	if handleStatSwapMove(p, defender, move, round, log) {
		return true
	}
	// Блок/форс ходов (block_moves.go): Овации/Подстрекательство/Запрет/Приказ/Обезоруживание.
	if handleBlockMove(p, defender, move, round, log) {
		return true
	}
	// Подготовительные ходы (focus_moves.go): Набат/Массаж/Заряд/Сброс балласта/
	// Прицел/Сосредоточение/Ультрафокусировка/Обнаружение/Чутьё/Третий глаз.
	if handleFocusMove(p, defender, move, round, log) {
		return true
	}
	// Жертвенные ходы (sacrifice_moves.go): Обморок/Связь судеб/Скорбная песнь.
	if handleSacrificeMove(p, defender, move, round, log) {
		return true
	}
	// Контратака (counter_moves.go): Контрудар — вернуть полученный урон ×2.
	if handleCounterMove(p, defender, move, round, log) {
		return true
	}
	// Выжидание (counter_moves.go): копит урон 2 хода, затем возвращает ×2.
	if handleBideMove(p, defender, move, round, log) {
		return true
	}
	// Статус-манипуляции (status_extra_moves.go): Зевок / Отместка.
	if handleStatusExtraMove(p, defender, move, round, log) {
		return true
	}
	// Электризация (field_effects.go): атака цели станет Электрической.
	if handleElectrify(p, defender, move, round, log) {
		return true
	}
	// Трансформация (transform_moves.go): стать копией противника.
	if handleTransformMove(p, defender, move, round, log) {
		return true
	}
	// Левитация (field_effects.go): цель иммунна к Земле + по ней всегда попадают.
	if handleLevitate(p, defender, move, round, log) {
		return true
	}
	// Лечащие кастом-ходы (heal_moves.go): Отдых/Укоренение/Водный покров/
	// Желание/Крик смерти/Очищение/Детокс/Силоотток + командное снятие статусов/
	// жертвенное возрождение/Запрет регенерации (нужен bs).
	return handleHealMove(bs, p, defender, move, round, log)
}

func stockpileUp(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p.Stockpile >= 3 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
			Message: p.Name + " не может накапливать больше!",
		})
		return
	}
	p.Stockpile++
	curseBump(p, 2, 1, round, log) // Защита +1
	curseBump(p, 5, 1, round, log) // СпЗащита +1
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " копит энергию! Запас: " + strconv.Itoa(p.Stockpile),
	})
}

func swallow(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p.Stockpile == 0 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: p.Name,
			Message: "Но запас пуст — не сработало!",
		})
		return
	}
	var heal int
	switch p.Stockpile {
	case 1:
		heal = p.MaxHP / 4
	case 2:
		heal = p.MaxHP / 2
	default:
		heal = p.MaxHP
	}
	if heal < 1 {
		heal = 1
	}
	p.CurrentHP += heal
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: p.Name, PokemonImage: p.ImageURL,
		Message: p.Name + " проглатывает запас и восстанавливает " + strconv.Itoa(heal) + " HP!",
	})
	consumeStockpile(p, round, log)
}

// consumeStockpile — обнулить запас и снизить Защиту/СпЗащиту на набранное.
func consumeStockpile(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	n := p.Stockpile
	if n <= 0 {
		return
	}
	curseBump(p, 2, -n, round, log) // Защита -n
	curseBump(p, 5, -n, round, log) // СпЗащита -n
	p.Stockpile = 0
}
