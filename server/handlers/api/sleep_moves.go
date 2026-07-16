package api

import mathrand "math/rand"

// sleep_moves.go — ходы, доступные ТОЛЬКО во сне: Храп(343) и Бормотание(457).
// Разрешение действовать во сне — в processTurn (sleepActingMove), сам сон тикает
// штатно в processPreTurnStatus.
//   343 Храп (Snore)       — урон-атака кат.2 (30% испуг — данными), провал не во сне.
//   457 Бормотание (Sleep Talk) — во сне применяет случайный СВОЙ ход.

const (
	moveSnore     = 343
	moveSleepTalk = 457
)

// sleepActingMove — ход, который покемон может использовать, находясь во сне.
func sleepActingMove(moveID int) bool {
	return moveID == moveSnore || moveID == moveSleepTalk
}

// sleepTalkMove — случайный СВОЙ ход для Бормотания (кроме вызывателей/себя и
// прочих ходов-вызывателей). nil, если подходящих нет.
func sleepTalkMove(p *BattlePokemon) *BattleMove {
	var pool []BattleMove
	for _, m := range p.Moves {
		if callerMoveIDs[m.ID] { // Бормотание/Метроном/Эскиз/… не выбираем
			continue
		}
		pool = append(pool, m)
	}
	if len(pool) == 0 {
		return nil
	}
	m := pool[mathrand.Intn(len(pool))]
	return &m
}
