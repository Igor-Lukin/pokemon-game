package api

// transform_moves.go — Трансформация (540, Transform): пользователь становится
// полной копией противника — перенимает его типы, базовые статы, ступени статов,
// набор атак и внешность. Собственный HP сохраняется. При смене — откат.
//
// Откат: типы (BattleTypes=nil) и базовые статы (restoreBaseStats) уже сбрасываются
// в местах смены; здесь дополнительно восстанавливаем набор атак, внешность и
// ступени из снимка (restoreTransform, зовётся из тех же мест сброса).

const moveTransform = 540

const transformedMovePP = 5 // Трансформация даёт по 5 PP на скопированный ход (канон)

// handleTransformMove — превратить attacker в defender. true, если ход обработан.
func handleTransformMove(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if move.ID != moveTransform {
		return false
	}
	if defender == nil {
		return true
	}
	if attacker.Transformed {
		*log = append(*log, BattleLogEntry{Round: round, PokemonName: attacker.Name, MoveFailed: "Уже в облике противника — не сработало!"})
		return true
	}
	// Снимок для отката при смене.
	attacker.Transformed = true
	attacker.SavedMoves = attacker.Moves
	attacker.SavedImageURL = attacker.ImageURL
	attacker.SavedStages = attacker.StatStages

	// Типы противника (через BattleTypes — перекрывает природный, сброс при смене).
	src := getPokemonTypes(defender)
	cp := make([]int, len(src))
	copy(cp, src)
	attacker.BattleTypes = cp

	// Базовые статы (кроме HP). Откат — restoreBaseStats из снимка Base* при смене.
	attacker.Attack = defender.Attack
	attacker.Defense = defender.Defense
	attacker.Speed = defender.Speed
	attacker.SpecialAttack = defender.SpecialAttack
	attacker.SpecialDefense = defender.SpecialDefense
	// Ступени статов и внешность.
	attacker.StatStages = defender.StatStages
	attacker.ImageURL = defender.ImageURL

	// Копия набора атак противника (каждой по 5 PP).
	moves := make([]BattleMove, len(defender.Moves))
	for i := range defender.Moves {
		moves[i] = defender.Moves[i]
		moves[i].PP = transformedMovePP
		moves[i].MaxPP = transformedMovePP
	}
	attacker.Moves = moves

	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: attacker.Name, PokemonImage: defender.ImageURL,
		Message: attacker.Name + " превращается в " + defender.Name + "!",
	})
	return true
}

// restoreTransform — откат Трансформации при уходе покемона: вернуть набор атак,
// внешность и ступени из снимка. Типы/базовые статы откатывают отдельно
// (BattleTypes=nil / restoreBaseStats).
func restoreTransform(p *BattlePokemon) {
	if !p.Transformed {
		return
	}
	p.Moves = p.SavedMoves
	p.ImageURL = p.SavedImageURL
	p.StatStages = p.SavedStages
	p.SavedMoves = nil
	p.Transformed = false
}
