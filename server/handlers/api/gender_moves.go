package api

import (
	mathrand "math/rand"
	"pokemon-game/server/db"
)

// gender_moves.go — ГЕНДЕРНЫЕ атаки (гейт по противоположному полу).
//
//   406 Обаяние (Captivate) — цели Спец.атака -2.
//   392 Соблазн (Attract)   — влюблённость (зеркало замешательства): перед
//                             действием ~50% шанс пропустить ход (без урона).
//
// Гейт oppositeGender требует И атакующего, И цель — поэтому это КАСТОМ по id,
// а не data-driven inflictVolatile (там цель без атакующего). Подключается в
// handleCustomStatusMove (stockpile_moves.go), где есть bs/attacker/defender.

// handleGenderMove — диспетчер гендерных атак по id. Возвращает true, если ход
// обработан здесь (тогда стандартная обработка статусного хода не нужна).
func handleGenderMove(p, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	switch move.ID {
	case 406: // Обаяние
		if oppositeGender(p, defender, round, log) {
			curseBump(defender, 4, -2, round, log) // Спец.атака (стат id 4) -2
		}
		return true
	case 392: // Соблазн
		if oppositeGender(p, defender, round, log) {
			inflictInfatuation(defender, round, log)
		}
		return true
	}
	return false
}

// oppositeGender — общий гейт «противоположный пол». Возвращает true, только
// если эффект применяется. Иначе пишет в лог «не действует» и возвращает false.
//   - у любого пол 'N' (или пусто) -> провал (бесполый неуязвим);
//   - полы одинаковые              -> провал;
//   - 'M' vs 'F'                    -> эффект применяется.
func oppositeGender(attacker, defender *BattlePokemon, round int, log *[]BattleLogEntry) bool {
	a, d := attacker.Gender, defender.Gender
	if a != db.GenderMale && a != db.GenderFemale {
		// сам атакующий бесполый — влиять не на что
		*log = append(*log, BattleLogEntry{
			Round:   round,
			Message: "Но на " + defender.Name + " это не действует!",
		})
		return false
	}
	if d != db.GenderMale && d != db.GenderFemale {
		*log = append(*log, BattleLogEntry{
			Round:   round,
			Message: "Но на " + defender.Name + " это не действует!",
		})
		return false
	}
	if a == d {
		*log = append(*log, BattleLogEntry{
			Round:   round,
			Message: "Но на " + defender.Name + " это не действует!",
		})
		return false
	}
	return true
}

// inflictInfatuation — накладывает влюблённость на цель (по образцу
// inflictConfusion). ВНИМАНИЕ: гейт oppositeGender должен быть пройден ДО вызова
// (сигнатура сознательно без атакующего — как у inflictVolatile).
func inflictInfatuation(t *BattlePokemon, round int, log *[]BattleLogEntry) {
	if t.Infatuated {
		return
	}
	t.Infatuated = true
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  t.Name,
		PokemonImage: t.ImageURL,
		StatusEvent:  "влюблённость",
	})
}

// volatileInfatuation — ПЕРЕД действием покемона (рядом с volatileConfusion).
// Если покемон влюблён — ~50% шанс, что «не может атаковать» и пропускает ход
// (возвращает false, БЕЗ урона). Иначе true. Влюблённость сама по себе не
// проходит по таймеру — снимается при уходе покемона (см. switch-хендлеры).
func volatileInfatuation(p *BattlePokemon, log *[]BattleLogEntry) bool {
	if !p.Infatuated {
		return true
	}
	*log = append(*log, BattleLogEntry{
		Round:       0,
		PokemonName: p.Name,
		Message:     p.Name + " влюблён(а)!",
	})
	if mathrand.Intn(100) < 50 {
		*log = append(*log, BattleLogEntry{
			Round:       0,
			PokemonName: p.Name,
			Message:     p.Name + " не может атаковать из-за влюблённости!",
		})
		return false
	}
	return true
}
