package api

import mathrand "math/rand"

// lock_moves.go — мультиход-локи: атака захватывает покемона и повторяется
// несколько раундов подряд (выбор игрока эти раунды игнорируется).
//   155 Ледяной шар (Rollout)  — до 5 раундов, мощность ×2 каждый раунд
//        (30→60→120→240→480), прерывается при ПРОМАХЕ.
//   21  Танец лепестков        — 2–3 раунда, по окончании САМО-замешательство.
//   348 Шум                    — 3 раунда.
//
// Состояние: BattlePokemon.LockedMoveID + LockTurns (осталось раундов).
// Принудительный выбор хода — в processTurn/chooseWildMoveOrCharge (как у
// зарядки). Нарастание мощности Ледяного шара — в escalatingPower по SameStreak.

type lockDef struct {
	minTurns         int  // минимум раундов (включая первый)
	maxTurns         int  // максимум раундов
	breakOnMiss      bool // прерывать лок при промахе (Rollout)
	confuseSelfAtEnd bool // само-замешательство по окончании (Танец лепестков)
}

var lockMoves = map[int]lockDef{
	155: {minTurns: 5, maxTurns: 5, breakOnMiss: true},      // Ледяной шар
	21:  {minTurns: 2, maxTurns: 3, confuseSelfAtEnd: true}, // Танец лепестков
	348: {minTurns: 3, maxTurns: 3},                         // Шум
}

// manageLock — обслуживание мультиход-лока после применения атаки. hit —
// попала ли атака (для прерывания Rollout при промахе). Вызывается в конце
// executeDamagingMove.
func manageLock(p *BattlePokemon, move *BattleMove, hit bool, round int, log *[]BattleLogEntry) {
	def, ok := lockMoves[move.ID]
	if !ok {
		return
	}
	// Начало лока (первое применение этой атаки).
	if p.LockedMoveID != move.ID {
		p.LockedMoveID = move.ID
		p.LockTurns = def.minTurns
		if def.maxTurns > def.minTurns {
			p.LockTurns += mathrand.Intn(def.maxTurns - def.minTurns + 1)
		}
	}
	p.LockTurns--

	end := p.LockTurns <= 0
	if def.breakOnMiss && !hit {
		end = true
	}
	if end {
		p.LockedMoveID = 0
		p.LockTurns = 0
		// Сброс нарастания, чтобы новый Ледяной шар начинался с 30.
		p.SameStreak = 0
		p.LastMoveID = 0
		if def.confuseSelfAtEnd && !p.IsFainted {
			inflictConfusion(p, round, log) // Танец лепестков — само-замешательство
		}
	}
}
