package api

// copy_moves.go — копирование/обмен модификаций характеристик (группа 3
// кластера «копирование/блок ходов»). Работают только со StatStages, новой
// инфры не требуют.
//   437 Пародия (Psych Up)      — копирует ВСЕ бусты статов противника себе.
//   512 Подмена атаки (Power Swap) — обмен ступеней Атаки(1)/СпАтаки(4) с противником.
//   513 Подмена защиты (Guard Swap) — обмен ступеней Защиты(2)/СпЗащиты(5) с противником.

const (
	movePsychUp    = 437
	movePowerSwap  = 512
	moveGuardSwap  = 513
	moveTopsyTurvy = 458 // инвертировать ступени статов противника
	moveHaze       = 496 // обнулить ступени статов ОБОИХ
)

// handleStatCopyMove — статусные ходы копирования/обмена стат-ступеней.
// Возвращает true, если ход обработан.
func handleStatCopyMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil {
		return false
	}
	switch move.ID {
	case movePsychUp:
		attacker.StatStages = defender.StatStages // копия всех ступеней
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " скопировал изменения характеристик противника!",
		})
	case movePowerSwap:
		swapStages(attacker, defender, 1, 4)
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " поменял местами модификации Атаки и Спец.Атаки с противником!",
		})
	case moveGuardSwap:
		swapStages(attacker, defender, 2, 5)
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + " поменял местами модификации Защиты и Спец.Защиты с противником!",
		})
	case moveTopsyTurvy: // Вверх тормашками — обратить все ступени противника
		for i := 1; i < len(defender.StatStages); i++ {
			defender.StatStages[i] = -defender.StatStages[i]
		}
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
			Message: "Все изменения характеристик " + defender.Name + " обращены в противоположные!",
		})
	case moveHaze: // Мгла — сбросить ступени статов ОБОИХ покемонов
		resetStages(attacker)
		resetStages(defender)
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: "Все изменения характеристик обоих монстров сброшены!",
		})
	default:
		return false
	}
	return true
}

// resetStages — обнулить ступени статов (индексы 1..7; 0 — HP, не трогаем).
func resetStages(p *BattlePokemon) {
	for i := 1; i < len(p.StatStages); i++ {
		p.StatStages[i] = 0
	}
}

// swapStages — обменять ступени статов idxs между двумя покемонами.
func swapStages(a, b *BattlePokemon, idxs ...int) {
	for _, i := range idxs {
		a.StatStages[i], b.StatStages[i] = b.StatStages[i], a.StatStages[i]
	}
}
