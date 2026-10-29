package api

// fire_effects.go — ослабление Огня. Пока один эффект:
//   Увлажнение(367, Water Sport) — ОБЩЕПОЛЕВОЙ (не сторона): сила всех огненных
//   атак ×0.33 (-67%) на 5 ходов. Живёт на BattleState (как погода, отдельный
//   таймер), НЕ на SideState — бьёт по Огню обеих сторон.
// (Огнеупорная пыльца(503) — отдельная механика/шаг: волатиль на покемоне.)

const (
	typeFire           = 2   // тип Огонь
	moveWaterSport     = 367 // Увлажнение
	waterSportDuration = 5   // раундов действия
	movePowder         = 503 // Огнеупорная пыльца (Powder)
)

// waterSportAdjust — если Увлажнение активно и ход огненного типа, возвращает
// КОПИЮ хода с мощностью ×0.33 (-67%). Иначе — тот же указатель без изменений.
// Тот же приём копии, что у weatherAdjustMove. Атаки с power 0 (сентинел/формула)
// остаются 0 — их формула не через move.Power.
func waterSportAdjust(move *BattleMove, active bool) *BattleMove {
	if !active || move.TypeID != typeFire || move.Power <= 0 {
		return move
	}
	m := *move
	m.Power = m.Power * 33 / 100
	if m.Power < 1 {
		m.Power = 1
	}
	return &m
}

// applyPowderMove — Огнеупорная пыльца: покрывает пыльцой ЦЕЛЬ (defender). Если
// цель в этот ход пойдёт огненной атакой — та провалится и ударит саму цель
// (firePowderBurst). Пыльца снимается в конце хода (действует текущий ход).
func applyPowderMove(defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	if defender == nil {
		return
	}
	// [batch6] Укрытие: порошок не прилипает.
	if defender.AbilityKey == "overcoat" {
		abilityLog(defender, round, log, "", "не пропускает порошок")
		return
	}
	defender.Powdered = true
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  defender.Name,
		PokemonImage: defender.ImageURL,
		Message:      defender.Name + " покрыт огнеупорной пыльцой!",
	})
}

// firePowderBurst — проверка перед выполнением хода: если покемон покрыт пыльцой
// и ходит ОГНЕННОЙ атакой, пыльца вспыхивает — атака проваливается, покемон
// теряет 1/4 макс. HP (канон Powder). Возвращает true, если ход сорван.
func firePowderBurst(attacker *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if !attacker.Powdered || move.TypeID != typeFire {
		return false
	}
	attacker.Powdered = false // сработала — снимаем
	dmg := attacker.MaxHP / 4
	if dmg < 1 {
		dmg = 1
	}
	attacker.CurrentHP -= dmg
	if attacker.CurrentHP < 0 {
		attacker.CurrentHP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  attacker.Name,
		PokemonImage: attacker.ImageURL,
		MoveName:     move.Name,
		MoveFailed:   "Пыльца вспыхнула — огненная атака провалилась!",
		Damage:       dmg,
	})
	if attacker.CurrentHP <= 0 {
		attacker.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round:       round,
			PokemonName: attacker.Name,
			Fainted:     true,
		})
	}
	return true
}
