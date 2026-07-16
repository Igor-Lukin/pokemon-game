package api

import (
	mathrand "math/rand"
	"strconv"
)

// special_dmg_moves.go — урон-атаки с особой логикой:
//   204 Подарок (Present)       — случайно: мощность 40/80/120 ИЛИ лечит цель 25%.
//   212 Последнее средство (Last Resort) — доступна, только когда все ОСТАЛЬНЫЕ
//       ходы покемона уже применялись в этом бою.

// applyPresentRoll — Подарок(204): бросок исхода. 20% — лечит ЦЕЛЬ на 25% макс. HP
// (урона нет); иначе возвращает копию хода со случайной мощностью 40/80/120.
// Второй результат true = ход отыгран лечением (урон не наносить).
func applyPresentRoll(move *BattleMove, defender *BattlePokemon, round int, log *[]BattleLogEntry) (*BattleMove, bool) {
	roll := mathrand.Intn(10)
	if roll < 2 { // 20% — «подарок» лечит противника
		heal := defender.MaxHP / 4
		if heal < 1 {
			heal = 1
		}
		if defender.CurrentHP >= defender.MaxHP {
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
				Message: "Подарок бесполезен — HP " + defender.Name + " уже полны!",
			})
			return nil, true
		}
		defender.CurrentHP += heal
		if defender.CurrentHP > defender.MaxHP {
			defender.CurrentHP = defender.MaxHP
		}
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
			Message: "Подарок оказался лечебным — " + defender.Name + " восстановил " + strconv.Itoa(heal) + " HP!",
		})
		return nil, true
	}
	m := *move
	switch {
	case roll < 6: // 40%
		m.Power = 40
	case roll < 9: // 30%
		m.Power = 80
	default: // 10%
		m.Power = 120
	}
	return &m, false
}

// markMoveUsed — запомнить, что покемон применил ход id: пополнить набор
// использованных (Последнее средство) и отметить как последний (Овации/Запрет/
// Подстрекательство/Приказ/Обезоруживание — block_moves.go).
func markMoveUsed(p *BattlePokemon, id int) {
	p.LastUsedMoveID = id
	for _, u := range p.UsedMoveIDs {
		if u == id {
			return
		}
	}
	p.UsedMoveIDs = append(p.UsedMoveIDs, id)
}

// lastResortReady — можно ли применить Последнее средство(212): у покемона есть
// другие ходы и ВСЕ они уже применялись в этом бою.
func lastResortReady(p *BattlePokemon) bool {
	others := 0
	for _, m := range p.Moves {
		if m.ID == 212 {
			continue
		}
		others++
		if !moveWasUsed(p, m.ID) {
			return false
		}
	}
	return others > 0
}

func moveWasUsed(p *BattlePokemon, id int) bool {
	for _, u := range p.UsedMoveIDs {
		if u == id {
			return true
		}
	}
	return false
}
